// Package promotion closes the advisory loop. The agent's own shadow-LLM
// pipeline generates thousands of policy suggestions and promotes almost
// none. This package provides the human side of the loop:
//
//  1. A human rejects the agent's action on a disagreement (review queue).
//  2. The rejection is bucketed by pattern (the adapter's PatternKey),
//     persistently — repeated rejections on the same spot class count up.
//  3. When a pattern crosses the threshold, a policy edit is auto-drafted
//     and staged for review (never applied automatically).
//  4. A human approves the draft, which yields a machine-applicable patch
//     for the agent's policy file, or discards it.
//
// The engine and the UI are domain-neutral; the pattern key, the drafted
// entry text, and its target section come from the adapter.
package promotion

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/sneldao/sekats/internal/engine"
)

// PatternState is one accumulated bucket of human rejections.
type PatternState struct {
	Key         string            `json:"key"`
	Count       int               `json:"count"`
	DecisionIDs []int64           `json:"decision_ids"`
	Evidence    []engine.Evidence `json:"evidence"`
	DraftID     string            `json:"draft_id,omitempty"`
}

// Store persists pattern buckets and staged drafts to disk (one JSON file
// each) and flushes on every write, so the pipeline state survives a
// restart — same durability contract as the verdicts store.
type Store struct {
	mu           sync.Mutex
	patternsPath string
	draftsPath   string
	// draftFactory builds a draft for a bucket; wired to the adapter.
	draftFactory func(key string, ev []engine.Evidence) engine.Draft

	patterns map[string]*PatternState
	drafts   map[string]engine.Draft
}

// NewStore returns a store persisting to the two given paths (a missing
// file means empty state; pass empty paths to keep state in memory only).
func NewStore(patternsPath, draftsPath string, draftFactory func(key string, ev []engine.Evidence) engine.Draft) *Store {
	return &Store{
		patternsPath: patternsPath,
		draftsPath:   draftsPath,
		draftFactory: draftFactory,
		patterns:     make(map[string]*PatternState),
		drafts:       make(map[string]engine.Draft),
	}
}

// Load reads previously-saved state. Missing files are not an error.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := loadJSON(s.patternsPath, &s.patterns); err != nil {
		return err
	}
	return loadJSON(s.draftsPath, &s.drafts)
}

// loadJSON unmarshals path into v; a missing (or empty) path/file is a
// no-op, so fresh checkouts start with empty pipeline state.
func loadJSON(path string, v any) error {
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// RecordRejection adds one human rejection to its pattern bucket. When the
// bucket crosses threshold (and no live draft already exists for it) a
// draft is auto-created, staged, and returned. Re-rejecting a decision
// already in the bucket is a no-op — the console must not count one hand
// twice. Returns nil when nothing was created.
func (s *Store) RecordRejection(key string, ev engine.Evidence, threshold int) *engine.Draft {
	s.mu.Lock()
	defer s.mu.Unlock()

	ps := s.patterns[key]
	if ps == nil {
		ps = &PatternState{Key: key}
		s.patterns[key] = ps
	}
	if containsID(ps.DecisionIDs, ev.DecisionID) {
		return nil // already counted
	}
	ps.Count++
	ps.DecisionIDs = append(ps.DecisionIDs, ev.DecisionID)
	ps.Evidence = append(ps.Evidence, ev)
	sort.Slice(ps.Evidence, func(i, j int) bool {
		return ps.Evidence[i].DecisionID < ps.Evidence[j].DecisionID
	})

	var created *engine.Draft
	if ps.Count >= threshold && s.needsDraft(ps) {
		d := s.draftFactory(key, append([]engine.Evidence{}, ps.Evidence...))
		if d.CreatedAt.IsZero() {
			d.CreatedAt = time.Now().UTC()
		}
		d.Status = engine.DraftPending
		s.drafts[d.ID] = d
		ps.DraftID = d.ID
		cp := d
		created = &cp
	}

	if err := s.flushLocked(); err != nil {
		logFlushErr(err)
	}
	if created != nil {
		c := *created
		return &c
	}
	return nil
}

// needsDraft reports whether the bucket currently warrants a (new) draft:
// no draft yet, or the existing one was discarded after more rejections
// came in (the pattern proved persistent, so it re-enters review).
func (s *Store) needsDraft(ps *PatternState) bool {
	if ps.DraftID == "" {
		return true
	}
	d, ok := s.drafts[ps.DraftID]
	if !ok {
		return true
	}
	if d.Status == engine.DraftDiscarded && ps.Count > len(d.Evidence) {
		return true
	}
	return false
}

// Draft returns one staged draft by ID.
func (s *Store) Draft(id string) (engine.Draft, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.drafts[id]
	return d, ok
}

// AllDrafts returns every draft, newest first.
func (s *Store) AllDrafts() []engine.Draft {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]engine.Draft, 0, len(s.drafts))
	for _, d := range s.drafts {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// Counts summarizes drafts by status.
func (s *Store) Counts() (pending, approved, discarded int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.drafts {
		switch d.Status {
		case engine.DraftPending:
			pending++
		case engine.DraftApproved:
			approved++
		case engine.DraftDiscarded:
			discarded++
		}
	}
	return
}

// Patterns returns every non-empty bucket, most rejections first.
func (s *Store) Patterns() []PatternState {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PatternState, 0, len(s.patterns))
	for _, ps := range s.patterns {
		if ps.Count > 0 {
			out = append(out, *ps)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// Approve moves a pending draft to approved (it now yields a patch).
// Approved and discarded drafts are terminal states.
func (s *Store) Approve(id string) (engine.Draft, error) {
	return s.resolve(id, engine.DraftApproved)
}

// Discard moves a pending draft to discarded.
func (s *Store) Discard(id string) (engine.Draft, error) {
	return s.resolve(id, engine.DraftDiscarded)
}

func (s *Store) resolve(id string, to string) (engine.Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.drafts[id]
	if !ok {
		return engine.Draft{}, fmt.Errorf("draft %s not found", id)
	}
	if d.Status != engine.DraftPending {
		return engine.Draft{}, fmt.Errorf("draft %s is %s, not pending", id, d.Status)
	}
	now := time.Now().UTC()
	d.Status = to
	d.ResolvedAt = &now
	s.drafts[id] = d
	if err := s.flushLocked(); err != nil {
		logFlushErr(err)
	}
	return d, nil
}

func (s *Store) flushLocked() error {
	if s.patternsPath != "" {
		if err := writeJSON(s.patternsPath, s.patterns); err != nil {
			return err
		}
	}
	if s.draftsPath != "" {
		if err := writeJSON(s.draftsPath, s.drafts); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func containsID(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func logFlushErr(err error) {
	// Flush failures mean state stays in memory only; the verdicts store
	// follows the same convention (report, don't crash the request).
	fmt.Fprintln(os.Stderr, "sekats: promotion state flush:", err)
}
