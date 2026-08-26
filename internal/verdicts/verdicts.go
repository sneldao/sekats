// Package verdicts persists the review queue's human verdicts to a JSON
// file on disk. Verdicts were in-memory only before; now the accept/reject
// calls a human makes in the queue survive a restart and feed the
// /verdicts report, so the queue's output is a durable artifact.
package verdicts

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Entry is one recorded human verdict.
type Entry struct {
	DecisionID int64
	Verdict    string // "accepted" (human agrees with the agent) or "rejected" (human agrees with the shadow-LLM)
	RecordedAt time.Time
}

// Store guards a map of decisionID -> Entry with a mutex and flushes the
// current state to a JSON file after every write.
type Store struct {
	mu       sync.Mutex
	path     string
	verdicts map[int64]Entry
}

// NewStore returns a store writing to path. Pass an empty path to disable
// persistence (verdicts stay in memory only).
func NewStore(path string) *Store {
	return &Store{path: path, verdicts: make(map[int64]Entry)}
}

// Load reads previously-saved verdicts from disk, if any. A missing file is
// not an error — the queue simply starts blank.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		return nil
	}
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	var saved map[string]Entry // JSON object keys are strings.
	if err := json.NewDecoder(f).Decode(&saved); err != nil {
		return err
	}
	for k, e := range saved {
		id, err := strconv.ParseInt(k, 10, 64)
		if err != nil {
			continue
		}
		s.verdicts[id] = e
	}
	return nil
}

// Record saves a verdict for the given decision ID and flushes to disk.
func (s *Store) Record(decisionID int64, verdict string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.verdicts[decisionID] = Entry{
		DecisionID: decisionID,
		Verdict:    verdict,
		RecordedAt: time.Now().UTC(),
	}
	return s.flushLocked()
}

// flushLocked writes the current map to disk. Caller holds s.mu.
func (s *Store) flushLocked() error {
	if s.path == "" {
		return nil
	}
	out := make(map[string]Entry, len(s.verdicts))
	for id, e := range s.verdicts {
		out[strconv.FormatInt(id, 10)] = e
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o644)
}

// All returns a snapshot of every recorded verdict, sorted by decision ID.
func (s *Store) All() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0, len(s.verdicts))
	for _, e := range s.verdicts {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DecisionID < out[j].DecisionID })
	return out
}

// Lookup returns the verdict for one decision ID, if recorded.
func (s *Store) Lookup(decisionID int64) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.verdicts[decisionID]
	return e, ok
}
