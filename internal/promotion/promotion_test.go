package promotion

import (
	"testing"
	"time"

	"github.com/sneldao/sekats/internal/engine"
)

func stubFactory(key string, ev []engine.Evidence) engine.Draft {
	return engine.Draft{
		ID:         key,
		PatternKey: key,
		Status:     engine.DraftPending,
		CreatedAt:  time.Now().UTC(),
	}
}

func TestRecordRejectionCreatesDraft(t *testing.T) {
	store := NewStore("", "", stubFactory)
	ev := evidence(1, "flop", "K5s")

	got := store.RecordRejection("x", ev, 3)
	if got != nil {
		t.Fatalf("first rejection should not draft (threshold=3)")
	}

	store.RecordRejection("x", evidence(2, "flop", "KTo"), 3)
	got = store.RecordRejection("x", evidence(3, "flop", "88"), 3)
	if got == nil {
		t.Fatal("third rejection should cross threshold=3")
	}
	if got.PatternKey != "x" {
		t.Fatalf("draft pattern key = %q", got.PatternKey)
	}
	if got.Status != engine.DraftPending {
		t.Fatalf("draft status = %q", got.Status)
	}
}

func TestRecordRejectionNoDuplicate(t *testing.T) {
	store := NewStore("", "", stubFactory)
	store.RecordRejection("x", evidence(1, "a", "b"), 3)
	got := store.RecordRejection("x", evidence(1, "a", "b"), 3)
	if got != nil {
		t.Fatal("second rejection of same decision should not count")
	}
}

func TestNeedsDraftLogic(t *testing.T) {
	store := NewStore("", "", stubFactory)

	// Below-threshold rejections build up
	store.RecordRejection("x", evidence(1, "a", "b"), 99)
	store.RecordRejection("x", evidence(2, "a", "b"), 99)

	// Stub factory always returns ID "stub"
	ps := store.patterns["x"]
	if ps == nil {
		t.Fatal("pattern state should exist")
	}
	if ps.Count != 2 {
		t.Fatalf("pattern count = %d", ps.Count)
	}

	// needsDraft is true when no draft ID set
	if !store.needsDraft(ps) {
		t.Fatal("pattern with no draft ID should need one")
	}

	// Stub ID but no draft in map: also needs
	ps.DraftID = "stub"
	if !store.needsDraft(ps) {
		t.Fatal("pattern with non-existent draft ID should need one")
	}

	// Draft exists and is pending: should not re-create
	store.drafts["stub"] = engine.Draft{
		ID:       "stub",
		Status:   engine.DraftPending,
		Evidence: []engine.Evidence{evidence(1, "a", "b")},
	}
	store.patterns["x"].DraftID = "stub"
	if store.needsDraft(ps) {
		t.Fatal("pending draft should not need re-creation")
	}

	// Discarded but new rejections added: should re-enter
	d := store.drafts["stub"]
	d.Status = engine.DraftDiscarded
	store.drafts["stub"] = d
	store.patterns["x"].Count = 2
	if !store.needsDraft(ps) {
		t.Fatal("discarded pattern with new rejections should re-enter")
	}

	// Discarded with no new rejections: stays quiet
	store.patterns["x"].Count = 1
	if store.needsDraft(ps) {
		t.Fatal("discarded pattern with no new rejections should not re-enter")
	}
}

func TestApprove(t *testing.T) {
	store := NewStore("", "", stubFactory)
	store.RecordRejection("x", evidence(1, "a", "b"), 1)
	d, err := store.Approve("x")
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != engine.DraftApproved {
		t.Fatalf("status = %q, want approved", d.Status)
	}
	if d.ResolvedAt == nil {
		t.Fatal("resolved_at should be set")
	}
}

func TestDiscard(t *testing.T) {
	store := NewStore("", "", stubFactory)
	store.RecordRejection("x", evidence(1, "a", "b"), 1)
	_, err := store.Discard("x")
	if err != nil {
		t.Fatal(err)
	}
}

func TestApproveNonPendingFails(t *testing.T) {
	store := NewStore("", "", stubFactory)
	store.RecordRejection("x", evidence(1, "a", "b"), 1)
	store.Discard("x")
	if _, err := store.Approve("x"); err == nil {
		t.Fatal("approve on discarded should fail")
	}
}

func TestUnknownDraftFails(t *testing.T) {
	store := NewStore("", "", stubFactory)
	if _, err := store.Approve("nope"); err == nil {
		t.Fatal("approve unknown draft should fail")
	}
	if _, err := store.Discard("nope"); err == nil {
		t.Fatal("discard unknown draft should fail")
	}
}

func TestPatternsSorted(t *testing.T) {
	store := NewStore("", "", stubFactory)
	store.RecordRejection("low", evidence(1, "a", "b"), 99)
	store.RecordRejection("low", evidence(2, "a", "b"), 99)
	store.RecordRejection("high", evidence(10, "a", "b"), 99)
	store.RecordRejection("high", evidence(20, "a", "b"), 99)
	store.RecordRejection("high", evidence(30, "a", "b"), 99)

	patterns := store.Patterns()
	if len(patterns) != 2 {
		t.Fatalf("patterns = %d, want 2", len(patterns))
	}
	if patterns[0].Key != "high" || patterns[0].Count != 3 {
		t.Fatalf("top pattern = %+v", patterns[0])
	}
	if patterns[1].Key != "low" || patterns[1].Count != 2 {
		t.Fatalf("second pattern = %+v", patterns[1])
	}
}

func TestCounts(t *testing.T) {
	store := NewStore("", "", stubFactory)
	store.RecordRejection("a", evidence(1, "a", "b"), 1)
	store.RecordRejection("b", evidence(2, "a", "b"), 1)
	store.RecordRejection("c", evidence(3, "a", "b"), 1)
	store.Approve("a")
	store.Discard("b")

	pending, approved, discarded := store.Counts()
	if pending != 1 || approved != 1 || discarded != 1 {
		t.Fatalf("counts = %d/%d/%d, want 1/1/1", pending, approved, discarded)
	}
}

func evidence(id int64, phase, code string) engine.Evidence {
	return engine.Evidence{
		DecisionID:  id,
		Phase:       phase,
		ContextCode: code,
		OurAction:   "bet",
		LLMAction:   "check",
		RecordedAt:  time.Now().UTC(),
	}
}
