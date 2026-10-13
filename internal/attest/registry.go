package attest

import "sort"

// Anchored returns every persisted successful anchor, ordered by draft ID.
// It does not depend on the current promotion queue or signer configuration.
func (s *Store) Anchored() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Record, 0, len(s.records))
	for _, r := range s.records {
		if r.Status != "anchored" {
			continue
		}
		if r.AnchoredAt != nil {
			at := *r.AnchoredAt
			r.AnchoredAt = &at
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DraftID < out[j].DraftID })
	return out
}
