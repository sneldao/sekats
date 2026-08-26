// Package review implements the human review queue: real hands where the
// agent's decision and the shadow-LLM review disagreed, with a human
// accept/reject verdict recorded against each one.
//
// The queue itself is in-memory (one per data snapshot); verdict
// durability lives in internal/verdicts, which the server re-applies to a
// freshly seeded queue after every restart or live-data refresh.
package review

import (
	"sync"

	"github.com/sneldao/sekats/internal/store"
)

// Verdict is the human's call on one disagreement: accept the agent's
// action as correct, or reject it in favor of the shadow-LLM's suggestion.
type Verdict string

const (
	VerdictPending  Verdict = "pending"
	VerdictAccepted Verdict = "accepted" // human agrees with the agent
	VerdictRejected Verdict = "rejected" // human agrees with the shadow-LLM
)

// Item pairs a disagreement fixture with its current human verdict.
type Item struct {
	store.Disagreement
	Verdict Verdict
}

// Queue holds the review items and guards concurrent access from HTTP
// handlers with a mutex (this is a small single-process demo server, not a
// distributed system — a mutex is the right amount of complexity here).
type Queue struct {
	mu    sync.RWMutex
	items []Item
}

// NewQueue seeds a queue from the disagreement fixtures, all starting
// pending.
func NewQueue(disagreements []store.Disagreement) *Queue {
	q := &Queue{items: make([]Item, 0, len(disagreements))}
	for _, d := range disagreements {
		q.items = append(q.items, Item{Disagreement: d, Verdict: VerdictPending})
	}
	return q
}

// All returns a snapshot of the current queue state.
func (q *Queue) All() []Item {
	q.mu.RLock()
	defer q.mu.RUnlock()
	out := make([]Item, len(q.items))
	copy(out, q.items)
	return out
}

// SetVerdict records a human decision for the item with the given
// decision ID. Returns false if no matching item was found.
func (q *Queue) SetVerdict(decisionID int64, v Verdict) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := range q.items {
		if q.items[i].DecisionID == decisionID {
			q.items[i].Verdict = v
			return true
		}
	}
	return false
}

// Counts returns how many items are in each verdict state.
func (q *Queue) Counts() (pending, accepted, rejected int) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	for _, it := range q.items {
		switch it.Verdict {
		case VerdictPending:
			pending++
		case VerdictAccepted:
			accepted++
		case VerdictRejected:
			rejected++
		}
	}
	return
}
