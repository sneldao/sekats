package policy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sneldao/sekats/internal/adapters"
	"github.com/sneldao/sekats/internal/adapters/poker"
	"github.com/sneldao/sekats/internal/store"
)

// A minimal poker policy + the real rules file shape, run through the
// adapter to prove the declarative pipeline (rules JSON + policy lists)
// reproduces the violation the old hard-coded rules caught.
const miniPolicy = `{
  "version": 1,
  "preflop": {
    "premium": ["AA", "KK"],
    "dominatedFoldCandidates": ["A7o", "K5o"],
    "trashExamples": ["72o"]
  }
}`

const miniRules = `[
  {"name": "dominated-hand-must-not-raise", "phase": "Preflop", "actions": ["raise"],
   "list_ref": "preflop.dominatedFoldCandidates", "detail": "{{code}} raised though dominated"},
  {"name": "trash-hand-must-not-call-or-raise", "phase": "Preflop", "actions": ["call", "raise"],
   "list_ref": "preflop.trashExamples", "detail": "{{code}} is trash but agent chose {{action}}"},
  {"name": "premium-hand-must-not-fold-preflop", "phase": "Preflop", "actions": ["fold"],
   "list_ref": "preflop.premium", "detail": "{{code}} is premium but folded preflop"}
]`

func setup(t *testing.T) (string, adapters.Adapter) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, poker.PolicyFileName), []byte(miniPolicy), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, poker.RulesFileName), []byte(miniRules), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, poker.New()
}

func TestRunCatchesViolations(t *testing.T) {
	dir, ad := setup(t)
	decisions := []store.Decision{
		{ID: 1, Street: "Preflop", HandCode: "A7o", Action: "raise", Pot: 5}, // violation
		{ID: 2, Street: "Preflop", HandCode: "72o", Action: "call", Pot: 3},  // violation
		{ID: 3, Street: "Preflop", HandCode: "AA", Action: "fold", Pot: 1},   // violation
		{ID: 4, Street: "Flop", HandCode: "A7o", Action: "raise", Pot: 8},    // no (phase)
		{ID: 5, Street: "Preflop", HandCode: "QQ", Action: "raise", Pot: 2},  // no (not in list)
	}
	rep, err := Run(ad, dir, decisions)
	if err != nil {
		t.Fatal(err)
	}
	if rep.TotalDecisions != 5 {
		t.Fatalf("total = %d", rep.TotalDecisions)
	}
	if len(rep.RulesRun) != 3 {
		t.Fatalf("rules run = %v", rep.RulesRun)
	}
	if len(rep.Violations) != 3 {
		t.Fatalf("violations = %d, want 3: %+v", len(rep.Violations), rep.Violations)
	}
	// Detail substitution uses the engine placeholders.
	for _, v := range rep.Violations {
		if v.Detail == "" {
			t.Fatalf("empty detail for rule %s", v.Rule)
		}
	}
}

func TestRunNoViolations(t *testing.T) {
	dir, ad := setup(t)
	decisions := []store.Decision{
		{ID: 1, Street: "Preflop", HandCode: "QQ", Action: "raise", Pot: 2},
		{ID: 2, Street: "Flop", HandCode: "72o", Action: "call", Pot: 3},
	}
	rep, err := Run(ad, dir, decisions)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Violations) != 0 {
		t.Fatalf("violations = %+v, want none", rep.Violations)
	}
}

func TestRunMissingRulesFile(t *testing.T) {
	dir, ad := setup(t)
	_ = os.Remove(filepath.Join(dir, poker.RulesFileName))
	if _, err := Run(ad, dir, nil); err == nil {
		t.Fatal("expected error when rules file is missing")
	}
}

func TestRunMissingPolicyFile(t *testing.T) {
	dir, ad := setup(t)
	_ = os.Remove(filepath.Join(dir, poker.PolicyFileName))
	if _, err := Run(ad, dir, nil); err == nil {
		t.Fatal("expected error when policy file is missing")
	}
}
