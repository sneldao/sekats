package attest

import (
	"strings"
	"testing"
	"time"

	"github.com/sneldao/sekats/internal/engine"
)

func approvedDraft() engine.Draft {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	return engine.Draft{
		ID:         "flop-bet-to-check",
		PatternKey: "flop: bet→check",
		Pattern:    "agent bets, shadow-LLM says check on flop",
		Rule:       "test rule",
		Target:     "postflop.promotedFromHandReviews",
		Payload:    map[string]any{"pattern": "flop: bet→check", "rule": "test rule"},
		Evidence: []engine.Evidence{
			{DecisionID: 102}, {DecisionID: 7}, {DecisionID: 55},
		},
		Status:     engine.DraftApproved,
		ResolvedAt: &now,
	}
}

func TestDocForRequiresApproval(t *testing.T) {
	d := approvedDraft()
	d.Status = engine.DraftPending
	if _, err := DocFor(d); err == nil {
		t.Fatal("expected error for pending draft")
	}
}

func TestDocCanonicalDeterministic(t *testing.T) {
	d := approvedDraft()
	doc, err := DocFor(d)
	if err != nil {
		t.Fatal(err)
	}
	b1, err := doc.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	// Rebuild from an identical draft — bytes must match exactly.
	doc2, err := DocFor(approvedDraft())
	if err != nil {
		t.Fatal(err)
	}
	b2, err := doc2.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) != string(b2) {
		t.Fatal("canonical JSON is not deterministic")
	}
	// Source decisions must be sorted ascending regardless of evidence order.
	if doc.SourceDecisions[0] != 7 || doc.SourceDecisions[2] != 102 {
		t.Fatalf("decisions not sorted: %v", doc.SourceDecisions)
	}
	if !strings.Contains(string(b1), "sekats-attestation/v1") {
		t.Fatal("doc missing schema")
	}
}

func TestDraftKeyAndHashStable(t *testing.T) {
	k := DraftKey("flop-bet-to-check")
	h := DocHash([]byte("hello"))
	if len(k) != 32 || len(h) != 32 {
		t.Fatal("bad digest length")
	}
	if DraftKey("a") == DraftKey("b") {
		t.Fatal("draft keys collide")
	}
}

func TestDecisionKeyDomainSeparated(t *testing.T) {
	k := DecisionKey(42)
	if len(k) != 32 {
		t.Fatal("bad digest length")
	}
	if DecisionKey(42) == DecisionKey(43) {
		t.Fatal("decision keys collide")
	}
	// A decision id must never collide with a draft id of the same digits.
	if DecisionKey(42) == DraftKey("42") {
		t.Fatal("decision/draft key spaces collide")
	}
}

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir + "/attestations.json")
	now := time.Now().UTC()
	r := Record{
		DraftID: "d1", DocSHA256: "ab", Chain: "Monad (chain 143)",
		TxHash: "0xdead", Status: "anchored", AnchoredAt: &now,
	}
	if err := s.Put(r); err != nil {
		t.Fatal(err)
	}
	s2 := NewStore(dir + "/attestations.json")
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	got, ok := s2.For("d1")
	if !ok || got.TxHash != "0xdead" || got.Status != "anchored" {
		t.Fatalf("round-trip failed: %+v", got)
	}
	if s2.AnchoredCount() != 1 {
		t.Fatalf("anchored count = %d", s2.AnchoredCount())
	}
	if _, ok := s2.For("missing"); ok {
		t.Fatal("unexpected record")
	}
}

func TestStoreMissingFileOK(t *testing.T) {
	s := NewStore(t.TempDir() + "/nope.json")
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
}
