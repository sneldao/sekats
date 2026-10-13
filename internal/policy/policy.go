// Package policy runs the agent's declarative acceptance criteria against
// logged decisions. The rules are data — a per-agent rules file (e.g.
// data/poker-rules.json), not Go code — loaded and validated by the
// engine, with the policy lists resolved by the domain adapter. This
// package is the thin bridge to the UI: it keeps the handler-facing
// Report shape stable.
package policy

import (
	"path/filepath"

	"github.com/sneldao/sekats/internal/adapters"
	"github.com/sneldao/sekats/internal/engine"
	"github.com/sneldao/sekats/internal/store"
)

// Report is the engine's rule report (alias so handler code and templates
// are unchanged by the move to declarative rules).
type Report = engine.Report

// Violation is one decision that broke one rule (alias).
type Violation = engine.Violation

// Run loads the adapter's policy doc and declarative rule set from dataDir
// and runs every decision against every rule.
func Run(a adapters.Adapter, dataDir string, decisions []store.Decision) (Report, error) {
	pol, err := a.LoadPolicy(filepath.Join(dataDir, a.PolicyFile()))
	if err != nil {
		return Report{}, err
	}
	rules, err := engine.LoadRules(filepath.Join(dataDir, a.RulesFile()))
	if err != nil {
		return Report{}, err
	}

	ds := make([]engine.Decision, len(decisions))
	for i, d := range decisions {
		ds[i] = d.EngineDecision()
	}
	return engine.RunRules(rules, pol, ds), nil
}
