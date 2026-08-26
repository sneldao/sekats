package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testPolicy(t *testing.T) *PolicyDoc {
	t.Helper()
	return NewPolicyDoc(1, []byte(`{"version":1}`), map[string][]string{
		"preflop.dominatedFoldCandidates": {"A7o", "K5o"},
		"preflop.premium":                 {"AA", "KK"},
		"preflop.trashExamples":           {"72o"},
	})
}

func testRules(t *testing.T) []Rule {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rules.json")
	err := os.WriteFile(path, []byte(`
[
  {"name": "dominated-must-not-raise", "phase": "Preflop", "actions": ["raise"],
   "list_ref": "preflop.dominatedFoldCandidates", "detail": "{{code}} raised though dominated"},
  {"name": "premium-must-not-fold", "phase": "Preflop", "actions": ["fold"],
   "list_ref": "preflop.premium", "detail": "{{code}} folded preflop"},
  {"name": "trash-must-not-call", "phase": "Preflop", "actions": ["call"],
   "list_ref": "preflop.trashExamples", "detail": "{{code}} called though trash"}
]`), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := LoadRules(path)
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

func TestRunRulesViolations(t *testing.T) {
	rules := testRules(t)
	pol := testPolicy(t)
	decisions := []Decision{
		{ID: 1, Phase: "Preflop", ContextCode: "A7o", Action: "raise", Pot: 5}, // violates dominated-must-not-raise
		{ID: 2, Phase: "Preflop", ContextCode: "AA", Action: "fold"},           // violates premium-must-not-fold
		{ID: 3, Phase: "Preflop", ContextCode: "72o", Action: "call"},          // violates trash-must-not-call
		{ID: 4, Phase: "Flop", ContextCode: "A7o", Action: "raise"},            // wrong phase: no violation
		{ID: 5, Phase: "Preflop", ContextCode: "A7o", Action: "call"},          // action not in rule: no violation
		{ID: 6, Phase: "Preflop", ContextCode: "QQ", Action: "raise"},          // code not in list: no violation
	}
	rep := RunRules(rules, pol, decisions)
	if rep.TotalDecisions != 6 {
		t.Fatalf("TotalDecisions = %d, want 6", rep.TotalDecisions)
	}
	if len(rep.RulesRun) != 3 {
		t.Fatalf("RulesRun = %v, want 3 rules", rep.RulesRun)
	}
	if len(rep.Violations) != 3 {
		t.Fatalf("Violations = %d, want 3: %+v", len(rep.Violations), rep.Violations)
	}
	want := map[string]string{
		"dominated-must-not-raise": "A7o raised though dominated",
		"premium-must-not-fold":    "AA folded preflop",
		"trash-must-not-call":      "72o called though trash",
	}
	for _, v := range rep.Violations {
		detail, ok := want[v.Rule]
		if !ok {
			t.Fatalf("unexpected rule %q in violations", v.Rule)
		}
		if v.Detail != detail {
			t.Fatalf("rule %s detail = %q, want %q", v.Rule, v.Detail, detail)
		}
		if strings.Contains(v.Detail, "{{") {
			t.Fatalf("unsubstituted placeholder in detail: %q", v.Detail)
		}
		delete(want, v.Rule)
	}
	if len(want) != 0 {
		t.Fatalf("missing violations for rules: %v", want)
	}
}

func TestRunRulesUnknownListRef(t *testing.T) {
	// A rule referencing a list the adapter did not resolve matches
	// nothing — it must not panic or false-positive.
	rules := []Rule{{Name: "x", Phase: "Preflop", Actions: []string{"raise"}, ListRef: "does.not.exist"}}
	rep := RunRules(rules, testPolicy(t), []Decision{{Phase: "Preflop", ContextCode: "A7o", Action: "raise"}})
	if len(rep.Violations) != 0 {
		t.Fatalf("violations = %v, want none", rep.Violations)
	}
}

func TestLoadRulesValidation(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`[{"name":"x"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRules(bad); err == nil {
		t.Fatal("expected error for rule without list_ref")
	}
	named := filepath.Join(dir, "named.json")
	if err := os.WriteFile(named, []byte(`[{"list_ref":"a.b"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRules(named); err == nil {
		t.Fatal("expected error for rule without name")
	}
	if _, err := LoadRules(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestExtractRationale(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`  "action": "check",\n  "rationale": "Check to see turn cheaply."\n}`, "Check to see turn cheaply."},
		{`{"rationale": "escaped \"quote\" here"}`, `escaped "quote" here`},
		{`{"action": "check"}`, ""},
		{"", ""},
		{"not json at all", ""},
	}
	for _, c := range cases {
		if got := ExtractRationale(c.in); got != c.want {
			t.Errorf("ExtractRationale(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPatch(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	d := Draft{
		ID:         "flop-bet-to-check",
		PatternKey: "flop: bet to check",
		Target:     "postflop.promotedFromHandReviews",
		Payload:    map[string]any{"pattern": "p", "rule": "r"},
		Evidence: []Evidence{
			{DecisionID: 11, Phase: "Flop", ContextCode: "K5s", OurAction: "bet", LLMAction: "check"},
			{DecisionID: 22, Phase: "Flop", ContextCode: "KTo", OurAction: "bet", LLMAction: "check"},
		},
		Status:    DraftApproved,
		CreatedAt: now,
	}
	p := d.Patch()
	if p.Schema != "sekats-policy-patch/v1" {
		t.Fatalf("schema = %q", p.Schema)
	}
	if p.DraftID != "flop-bet-to-check" || p.Target != "postflop.promotedFromHandReviews" {
		t.Fatalf("patch identity wrong: %+v", p)
	}
	if len(p.SourceDecisions) != 2 || p.SourceDecisions[0] != 11 || p.SourceDecisions[1] != 22 {
		t.Fatalf("source decisions = %v", p.SourceDecisions)
	}
	if p.Entry["rule"] != "r" {
		t.Fatalf("entry = %v", p.Entry)
	}
}
