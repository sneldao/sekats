package poker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sneldao/sekats/internal/engine"
)

const samplePolicy = `{
  "version": 7,
  "preflop": {
    "premium": ["AA", "KK", "QQ"],
    "strong": ["TT"],
    "playableSuited": ["ATs"],
    "playableOffsuitLate": ["AJo"],
    "dominatedFoldCandidates": ["A2o", "A7o", "K5o"],
    "trashExamples": ["72o", "93o"]
  },
  "postflop": {
    "promotedFromHandReviews": []
  }
}`

func TestLoadPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "poker-policy.json")
	if err := os.WriteFile(path, []byte(samplePolicy), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := New().LoadPolicy(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Version != 7 {
		t.Fatalf("version = %d, want 7", doc.Version)
	}
	if len(doc.Raw) == 0 {
		t.Fatal("raw policy bytes not preserved")
	}
	list, ok := doc.List("preflop.dominatedFoldCandidates")
	if !ok || len(list) != 3 {
		t.Fatalf("dominatedFoldCandidates = %v (ok=%v), want 3 entries", list, ok)
	}
	if _, ok := doc.List("preflop.unknown"); ok {
		t.Fatal("unknown ref should not resolve")
	}
	if _, err := New().LoadPolicy(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected error for missing policy file")
	}
}

func TestPatternKey(t *testing.T) {
	a := New()
	cases := []struct {
		r    engine.Review
		want string
	}{
		{engine.Review{Phase: "Flop", OurAction: "bet", LLMAction: "check"}, "flop: bet to check"},
		{engine.Review{Phase: "Turn", OurAction: "bet", LLMAction: "check"}, "turn: bet to check"},
		{engine.Review{Phase: "Preflop", OurAction: "raise", LLMAction: "fold"}, "preflop: raise to fold"},
		{engine.Review{Phase: "  ", OurAction: "bet", LLMAction: "check"}, "unknown: bet to check"},
	}
	for _, c := range cases {
		if got := a.PatternKey(c.r); got != c.want {
			t.Errorf("PatternKey(%+v) = %q, want %q", c.r, got, c.want)
		}
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"flop: bet to check":       "flop-bet-to-check",
		"turn: raise to fold":      "turn-raise-to-fold",
		"preflop:  raise  to call": "preflop-raise-to-call",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewDraft(t *testing.T) {
	a := New()
	ev := []engine.Evidence{
		{DecisionID: 10, Phase: "Flop", ContextCode: "K5s", OurAction: "bet", LLMAction: "check", Pot: 12, Rationale: "weak draw"},
		{DecisionID: 20, Phase: "Flop", ContextCode: "KTo", OurAction: "bet", LLMAction: "check", Pot: 13, Rationale: "top pair"},
		{DecisionID: 30, Phase: "Flop", ContextCode: "K5s", OurAction: "bet", LLMAction: "check", Pot: 26, Rationale: "multiway"},
	}
	d := a.NewDraft("flop: bet to check", ev)

	if d.ID != "flop-bet-to-check" {
		t.Fatalf("id = %q", d.ID)
	}
	if d.Target != PolicyTarget {
		t.Fatalf("target = %q", d.Target)
	}
	if d.Status != engine.DraftPending {
		t.Fatalf("status = %q", d.Status)
	}
	if !strings.Contains(d.Pattern, "3 times") {
		t.Fatalf("pattern should count rejections: %q", d.Pattern)
	}
	for _, code := range []string{"K5s", "KTo"} {
		if !strings.Contains(d.Pattern, code) {
			t.Fatalf("pattern should name representative hands: %q", d.Pattern)
		}
	}
	if !strings.Contains(d.Rule, "check") || !strings.Contains(d.Rule, "bet") {
		t.Fatalf("rule should name the endorsed action: %q", d.Rule)
	}
	if len(d.Evidence) != 3 {
		t.Fatalf("evidence = %d, want 3", len(d.Evidence))
	}
	// The payload is exactly what lands in the policy file.
	if d.Payload["pattern"] == "" || d.Payload["rule"] == "" {
		t.Fatalf("payload = %v", d.Payload)
	}
	// NewDraft must not alias the caller's slice: mutating ev later must
	// not corrupt the stored draft.
	ev[0].ContextCode = "TAMPERED"
	if d.Evidence[0].ContextCode != "K5s" {
		t.Fatal("draft aliases the caller's evidence slice")
	}
}
