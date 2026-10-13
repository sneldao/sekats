// Package attest anchors human-approved policy edits on Monad.
//
// When a policy draft is approved, the console builds a canonical
// attestation document (the exact draft contents a human signed off on),
// hashes it with SHA-256, and writes that digest into the
// SekatsAttestations contract on-chain. The console itself never holds
// funds and never mutates the agent — the anchor is a one-way proof that
// "this exact policy change was approved, by this attester, at this
// time", verifiable by anyone who re-hashes the served document.
//
// Signing is delegated to `cast send` (Foundry): sekats stays
// dependency-light and the private key lives only in the operator's
// environment, never in the repo or in the data files. When no signer is
// configured the feature degrades honestly — approvals still work, the
// UI says anchoring is unconfigured.
package attest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sneldao/sekats/internal/engine"
)

// Record is the durable outcome of anchoring one approved draft.
type Record struct {
	DraftID    string     `json:"draft_id"`
	DocSHA256  string     `json:"doc_sha256"` // sha256 hex of the canonical Doc
	Chain      string     `json:"chain"`
	Contract   string     `json:"contract,omitempty"`
	TxHash     string     `json:"tx_hash,omitempty"`
	Attester   string     `json:"attester,omitempty"`
	Status     string     `json:"status"` // "anchored" or "failed"
	Error      string     `json:"error,omitempty"`
	AnchoredAt *time.Time `json:"anchored_at,omitempty"`
}

// Doc is the canonical attestation payload — the preimage of the
// on-chain hash. It is byte-stable: every field is fixed at approval
// time and MarshalIndent emits map keys sorted, so re-serving the doc
// produces identical bytes and an identical digest.
type Doc struct {
	Schema          string         `json:"schema"` // "sekats-attestation/v1"
	DraftID         string         `json:"draft_id"`
	PatternKey      string         `json:"pattern_key"`
	Target          string         `json:"target"`
	Entry           map[string]any `json:"entry"`
	SourceDecisions []int64        `json:"source_decisions"`
	ApprovedAt      time.Time      `json:"approved_at"`
}

// DocFor builds the canonical attestation doc for an approved draft.
func DocFor(d engine.Draft) (Doc, error) {
	if d.Status != engine.DraftApproved || d.ResolvedAt == nil {
		return Doc{}, fmt.Errorf("draft %s is not approved", d.ID)
	}
	ids := make([]int64, 0, len(d.Evidence))
	for _, e := range d.Evidence {
		ids = append(ids, e.DecisionID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return Doc{
		Schema:          "sekats-attestation/v1",
		DraftID:         d.ID,
		PatternKey:      d.PatternKey,
		Target:          d.Target,
		Entry:           d.Payload,
		SourceDecisions: ids,
		ApprovedAt:      d.ResolvedAt.UTC(),
	}, nil
}

// CanonicalJSON renders the doc byte-stably.
func (d Doc) CanonicalJSON() ([]byte, error) {
	return json.MarshalIndent(d, "", "  ")
}

// DraftKey derives the contract's bytes32 key for a draft id.
// Domain-separated so an attacker can't pick a draft id that collides
// with some other application's key space.
func DraftKey(draftID string) [32]byte {
	return sha256.Sum256([]byte("sekats/draft/" + draftID))
}

// DocHash is the digest anchored on-chain.
func DocHash(doc []byte) [32]byte { return sha256.Sum256(doc) }

// Anchorer submits attestation transactions. Implementations must not
// retain key material beyond the lifetime of one call.
type Anchorer interface {
	// Anchor writes docHash under draftKey; returns tx hash and attester.
	Anchor(ctx context.Context, draftKey, docHash [32]byte) (txHash, attester string, err error)
	// ChainName is the display label, e.g. "Monad (chain 143)".
	ChainName() string
	// Contract is the on-chain registry address.
	Contract() string
	// ExplorerTX renders an explorer URL for a tx, "" when no explorer
	// is configured.
	ExplorerTX(txHash string) string
}

// Store persists one Record per draft, flushed on every write — same
// durability contract as the verdicts and promotion stores.
type Store struct {
	mu      sync.Mutex
	path    string
	records map[string]Record
}

// NewStore persists to path; an empty path keeps state in memory only.
func NewStore(path string) *Store {
	return &Store{path: path, records: map[string]Record{}}
}

// Load reads previously-saved records; a missing file means empty state.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		return nil
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(raw, &s.records)
}

// For returns the record for one draft.
func (s *Store) For(draftID string) (Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[draftID]
	return r, ok
}

// AnchoredCount reports how many drafts carry a confirmed anchor.
func (s *Store) AnchoredCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.records {
		if r.Status == "anchored" {
			n++
		}
	}
	return n
}

// Put records the outcome durably.
func (s *Store) Put(r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[r.DraftID] = r
	if s.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(s.records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}

// FromEnv resolves anchoring config. Returns nil (disabled) when
// SEKATS_ATTEST=off or when key/contract/cast are missing — the console
// still works, it just doesn't anchor.
func FromEnv() (Anchorer, error) {
	if os.Getenv("SEKATS_ATTEST") == "off" {
		return nil, nil
	}
	key := os.Getenv("SEKATS_ATTEST_KEY")
	contract := os.Getenv("SEKATS_ATTEST_CONTRACT")
	if key == "" || contract == "" {
		return nil, nil
	}
	bin := envOr("CAST_BIN", "cast")
	if _, err := exec.LookPath(bin); err != nil {
		return nil, fmt.Errorf("cast not found (%s): install Foundry to enable anchoring", bin)
	}
	return &CastAnch{
		RPC:          envOr("SEKATS_RPC", "https://rpc.monad.xyz"),
		ContractAddr: contract,
		Key:          key,
		Bin:          bin,
		Chain:        "Monad (chain 143)",
		Explorer:     envOr("SEKATS_EXPLORER", "https://monadscan.com"),
	}, nil
}

// CastAnch anchors via `cast send`. The key is passed to cast's argv for
// the duration of the call only — use a dedicated low-balance attester
// key, never the agent's wallet key.
type CastAnch struct {
	RPC          string
	ContractAddr string
	Key          string
	Bin          string
	Chain        string
	Explorer     string
}

func (c *CastAnch) ChainName() string { return c.Chain }
func (c *CastAnch) Contract() string  { return c.ContractAddr }
func (c *CastAnch) ExplorerTX(tx string) string {
	if c.Explorer == "" || tx == "" {
		return ""
	}
	return strings.TrimRight(c.Explorer, "/") + "/tx/" + tx
}

// castReceipt is the subset of `cast send --json` output we need.
type castReceipt struct {
	TransactionHash string `json:"transactionHash"`
	From            string `json:"from"`
	Status          string `json:"status"`
}

// Anchor submits anchor(draftKey, docHash) and waits for the receipt.
func (c *CastAnch) Anchor(ctx context.Context, draftKey, docHash [32]byte) (string, string, error) {
	args := []string{
		"send", c.ContractAddr,
		"anchor(bytes32,bytes32)",
		"0x" + hex.EncodeToString(draftKey[:]),
		"0x" + hex.EncodeToString(docHash[:]),
		"--rpc-url", c.RPC,
		"--private-key", c.Key,
		"--json",
	}
	cmd := exec.CommandContext(ctx, c.Bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("cast send: %v — %s", err, strings.TrimSpace(stderr.String()))
	}
	var rc castReceipt
	if err := json.Unmarshal(stdout.Bytes(), &rc); err != nil {
		return "", "", fmt.Errorf("parse cast receipt: %v — raw: %.200s", err, stdout.String())
	}
	if rc.Status != "0x1" {
		return rc.TransactionHash, rc.From, fmt.Errorf("tx %s reverted", rc.TransactionHash)
	}
	return rc.TransactionHash, rc.From, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
