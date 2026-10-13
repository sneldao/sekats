package attest

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAnchoredLoadsAllPersistedRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attestations.json")
	s := NewStore(path)
	at := time.Now().UTC()
	for _, rec := range []Record{
		{DraftID: "orphan", Status: "anchored", AnchoredAt: &at},
		{DraftID: "approved", Status: "anchored"},
		{DraftID: "failed", Status: "failed"},
	} {
		if err := s.Put(rec); err != nil {
			t.Fatal(err)
		}
	}
	s = NewStore(path)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	got := s.Anchored()
	if len(got) != 2 || got[0].DraftID != "approved" || got[1].DraftID != "orphan" {
		t.Fatalf("records = %+v", got)
	}
	got[0].Status = "failed"
	*got[1].AnchoredAt = time.Time{}
	if rec, _ := s.For("orphan"); rec.AnchoredAt.IsZero() || s.AnchoredCount() != 2 {
		t.Fatal("registry snapshot mutated stored state")
	}
	if empty := NewStore("").Anchored(); empty == nil || len(empty) != 0 {
		t.Fatalf("empty records = %#v", empty)
	}
}
