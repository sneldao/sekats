// Package jury reconstructs the juror roster from the SekatsJury
// contract's JurorVerdict event logs — the chain is the source of truth,
// not any local store.
//
// Public Monad RPCs cap eth_getLogs at ~100 blocks, so the Watcher walks
// the range incrementally in the background with a persisted cursor: it
// catches up over a bounded backlog, then keeps pace with the head.
// Point SEKATS_INDEX_RPC at an indexer-grade endpoint (Envio HyperRPC,
// Alchemy) to raise the window and converge faster — same code path.
package jury

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sneldao/sekats/internal/enrich"
	"github.com/sneldao/sekats/internal/monad"
)

// JurorVerdict(bytes32,address,uint8)
const verdictEventTopic = "0x41693246f41030167d0431739c029da7ee14807d19c72d1462e29adcd3c463bb"

const (
	snapshotTTL  = 30 * time.Second
	catchUpTick  = 800 * time.Millisecond // per chunk while behind head
	headTick     = 15 * time.Second       // idle pace once caught up
	defaultDepth = 50000                  // initial lookback when no fromBlock
)

// Juror is one wallet that has voted, with its current side tallies and
// optional Nansen labels.
type Juror struct {
	Address      string         `json:"address"`
	AgentVotes   int            `json:"agent_votes"`
	ShadowVotes  int            `json:"shadow_votes"`
	Labels       []enrich.Label `json:"labels,omitempty"`
	LabelsSource string         `json:"labels_source,omitempty"`
}

// Roster is the cached view served to the console. ScannedTo lags Head
// while the indexer is still catching up — callers may show indexing
// progress but the listed jurors are always real chain data.
type Roster struct {
	Jurors    []Juror `json:"jurors"`
	Source    string  `json:"source"` // "chain" | "unavailable" | "unconfigured"
	ScannedTo uint64  `json:"scanned_to,omitempty"`
	Head      uint64  `json:"head,omitempty"`
	CheckedAt string  `json:"checked_at"`
}

// verdict is one latest-verdict record, keyed "juror|decisionKey".
type verdict struct {
	V uint8  `json:"v"` // 1 = agent, 2 = shadow
	S uint64 `json:"s"` // packed block<<32 | logIndex
}

type persistedState struct {
	Cursor uint64             `json:"cursor"`
	Seen   map[string]verdict `json:"seen"`
}

// Watcher tails the contract's event log. file is the persisted cursor +
// verdict map path ("" = memory only). fromBlock bounds the initial scan;
// zero means "recent history only" (defaultDepth behind head).
type Watcher struct {
	client    *monad.Client
	contract  string
	fromBlock uint64
	enricher  *enrich.Client
	file      string

	mu        sync.Mutex
	seen      map[string]verdict
	cursor    uint64 // next block to scan
	scannedTo uint64 // furthest block actually covered
	head      uint64
	rpcUp     bool
	dirty     bool

	cached Roster
	expiry time.Time
}

func NewWatcher(client *monad.Client, contract string, fromBlock uint64, enricher *enrich.Client, file string) *Watcher {
	return &Watcher{
		client: client, contract: contract, fromBlock: fromBlock,
		enricher: enricher, file: file, seen: map[string]verdict{},
	}
}

// Snapshot returns the cached roster — never blocks on RPC. Jurors are
// built from whatever the indexer has folded in so far.
func (w *Watcher) Snapshot(ctx context.Context) Roster {
	if w == nil || w.client == nil || !monad.ValidAddress(w.contract) {
		return Roster{Source: "unconfigured", CheckedAt: now()}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if time.Now().Before(w.expiry) {
		return w.cached
	}
	r := w.buildLocked(ctx)
	w.cached, w.expiry = r, time.Now().Add(snapshotTTL)
	return r
}

// Start tails the chain until ctx is cancelled: one bounded scan chunk per
// tick while behind head, slower once caught up.
func (w *Watcher) Start(ctx context.Context) {
	if w == nil || w.client == nil || !monad.ValidAddress(w.contract) {
		return
	}
	w.load()
	for {
		caughtUp := w.step(ctx)
		w.persist()
		wait := headTick
		if !caughtUp {
			wait = catchUpTick
		}
		select {
		case <-ctx.Done():
			w.persist()
			return
		case <-time.After(wait):
		}
	}
}

// step scans one window and folds any verdicts found. Returns true when the
// cursor has reached head (idle pace appropriate).
func (w *Watcher) step(ctx context.Context) bool {
	head, err := w.client.BlockNumber(ctx)
	w.mu.Lock()
	if err != nil {
		w.rpcUp = false
		w.mu.Unlock()
		return true
	}
	w.rpcUp, w.head = true, head
	if w.cursor == 0 {
		if w.fromBlock > 0 && w.fromBlock <= head {
			w.cursor = w.fromBlock
		} else if head > defaultDepth {
			w.cursor = head - defaultDepth
		} else {
			w.cursor = 1 // genesis-inclusive start
		}
	}
	chunk := w.client.LogChunk()
	from, to := w.cursor, head
	if to > from+chunk-1 {
		to = from + chunk - 1
	}
	w.mu.Unlock()

	logs, err := w.client.Logs(ctx, w.contract, verdictEventTopic, from, to)
	if err != nil {
		w.mu.Lock()
		w.rpcUp = false
		w.mu.Unlock()
		return true
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	for _, l := range logs {
		if len(l.Topics) != 3 || len(l.Topics[1]) != 66 || len(l.Topics[2]) != 66 {
			continue
		}
		juror := "0x" + strings.ToLower(l.Topics[2][26:])
		block, _ := parseQty(l.BlockNumber)
		idx, _ := parseQty(l.Index)
		k := juror + "|" + strings.ToLower(l.Topics[1])
		stamp := block<<32 | idx
		if cur, ok := w.seen[k]; ok && stamp < cur.S {
			continue
		}
		if v := parseVerdict(l.Data); v == 1 || v == 2 {
			w.seen[k] = verdict{V: v, S: stamp}
			w.dirty = true
		}
	}
	w.cursor, w.scannedTo = to+1, to
	w.dirty = true // cursor advanced even when no verdicts found
	w.expiry = time.Time{} // roster changed — next Snapshot rebuilds
	return w.cursor > head
}

func (w *Watcher) buildLocked(ctx context.Context) Roster {
	if !w.rpcUp && w.scannedTo == 0 {
		return Roster{Source: "unavailable", CheckedAt: now()}
	}
	counts := map[string]*Juror{}
	for k, v := range w.seen {
		juror := k[:strings.IndexByte(k, '|')]
		j := counts[juror]
		if j == nil {
			j = &Juror{Address: juror}
			counts[juror] = j
		}
		if v.V == 1 {
			j.AgentVotes++
		} else {
			j.ShadowVotes++
		}
	}
	jurors := make([]Juror, 0, len(counts))
	for _, j := range counts {
		jurors = append(jurors, *j)
	}
	sort.Slice(jurors, func(a, b int) bool {
		return jurors[a].AgentVotes+jurors[a].ShadowVotes > jurors[b].AgentVotes+jurors[b].ShadowVotes
	})
	if w.enricher != nil {
		for i := range jurors {
			r := w.enricher.Lookup(ctx, jurors[i].Address)
			if len(r.Labels) > 0 {
				jurors[i].Labels = r.Labels
				jurors[i].LabelsSource = r.Source
			}
		}
	}
	return Roster{Jurors: jurors, Source: "chain", ScannedTo: w.scannedTo, Head: w.head, CheckedAt: now()}
}

func (w *Watcher) persist() {
	if w.file == "" {
		return
	}
	w.mu.Lock()
	if !w.dirty {
		w.mu.Unlock()
		return
	}
	state := persistedState{Cursor: w.cursor, Seen: w.seen}
	w.dirty = false
	w.mu.Unlock()

	raw, err := json.Marshal(state)
	if err != nil {
		return
	}
	tmp := w.file + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return
	}
	os.Rename(tmp, w.file)
}

func (w *Watcher) load() {
	if w.file == "" {
		return
	}
	raw, err := os.ReadFile(w.file)
	if err != nil {
		return
	}
	var state persistedState
	if json.Unmarshal(raw, &state) != nil {
		return
	}
	w.mu.Lock()
	w.seen, w.cursor = state.Seen, state.Cursor
	if w.seen == nil {
		w.seen = map[string]verdict{}
	}
	w.mu.Unlock()
}

func parseQty(hexQty string) (uint64, error) {
	return strconv.ParseUint(strings.TrimPrefix(hexQty, "0x"), 16, 64)
}

func parseVerdict(data string) uint8 {
	data = strings.TrimPrefix(data, "0x")
	if len(data) < 64 {
		return 0
	}
	switch data[len(data)-1] {
	case '1':
		return 1
	case '2':
		return 2
	}
	return 0
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }
