// Package policy turns the agent's own poker-policy.json into checkable
// assertions ("acceptance criteria") and runs them against real decisions.
package policy

import (
	"fmt"
	"strings"

	"github.com/udingethe/sekats/internal/store"
)

// Violation is one decision that broke one policy rule.
type Violation struct {
	Rule     string
	HandCode string
	Street   string
	Action   string
	Pot      float64
	Detail   string
}

// Rule is a single named assertion that can be run against a decision.
type Rule struct {
	Name  string
	Check func(store.Decision, store.Policy) (bool, string) // (violated, detail)
}

// Rules is the fixed set of policy-as-AC checks for the preflop street.
// Each rule encodes one claim from docs/poker-strategy.md / poker-policy.json
// as something a program can actually check against a logged decision.
var Rules = []Rule{
	{
		Name: "dominated-hand-must-not-raise",
		Check: func(d store.Decision, p store.Policy) (bool, string) {
			if d.Street != "Preflop" || d.Action != "raise" {
				return false, ""
			}
			if contains(p.Preflop.DominatedFoldCandidates, d.HandCode) {
				return true, fmt.Sprintf("%s is listed as a dominatedFoldCandidate but the agent raised", d.HandCode)
			}
			return false, ""
		},
	},
	{
		Name: "trash-hand-must-not-call-or-raise",
		Check: func(d store.Decision, p store.Policy) (bool, string) {
			if d.Street != "Preflop" || (d.Action != "call" && d.Action != "raise") {
				return false, ""
			}
			if contains(p.Preflop.TrashExamples, d.HandCode) {
				return true, fmt.Sprintf("%s is a trashExamples hand but the agent chose %s", d.HandCode, d.Action)
			}
			return false, ""
		},
	},
	{
		Name: "premium-hand-must-not-fold-preflop",
		Check: func(d store.Decision, p store.Policy) (bool, string) {
			if d.Street != "Preflop" || d.Action != "fold" {
				return false, ""
			}
			if contains(p.Preflop.Premium, d.HandCode) {
				return true, fmt.Sprintf("%s is a premium hand but the agent folded preflop", d.HandCode)
			}
			return false, ""
		},
	},
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

// Report is the result of running all Rules against a set of decisions.
type Report struct {
	TotalDecisions int
	Violations     []Violation
	RulesRun       []string
}

// Run checks every decision against every rule and collects violations.
func Run(decisions []store.Decision, p store.Policy) Report {
	r := Report{TotalDecisions: len(decisions)}
	for _, rule := range Rules {
		r.RulesRun = append(r.RulesRun, rule.Name)
	}
	for _, d := range decisions {
		for _, rule := range Rules {
			violated, detail := rule.Check(d, p)
			if violated {
				r.Violations = append(r.Violations, Violation{
					Rule:     rule.Name,
					HandCode: d.HandCode,
					Street:   d.Street,
					Action:   d.Action,
					Pot:      d.Pot,
					Detail:   detail,
				})
			}
		}
	}
	return r
}
