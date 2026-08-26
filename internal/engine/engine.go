// Package engine is the domain-neutral core of sekats: policy-as-ACs,
// shadow-review linking, and the human-in-the-loop promotion pipeline.
//
// Nothing in this package knows about poker. Domain knowledge — which policy
// sections count as checkable lists, what a "pattern" means, what the
// drafted policy edit should say — lives in internal/adapters. The same
// engine drives any autonomous agent that has a policy file and a decision
// log.
package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Decision is one row from an agent's decision log. The fields are generic
// on purpose: Phase is "Preflop"/"Flop"/... for the poker agent but is just
// "phase" for any agent; ContextCode is "A7o" for poker but whatever
// situation tag the agent logs (ticket ID, tool name, input class, ...).
type Decision struct {
	ID          int64
	DecidedAt   int64 // epoch ms
	Phase       string
	ContextCode string
	Action      string
	Amount      *float64
	Pot         float64
	Source      string
	Accepted    int
	Error       *string
}

// Review is one shadow-review row linked 1:1 to a decision, carrying the
// agent's actual action and the reviewer's proposed action.
type Review struct {
	DecisionID   int64
	Phase        string
	ContextCode  string
	OurAction    string
	LLMAction    string
	Pot          float64
	ResponseText string
}

// PolicyDoc is an agent's policy file: the original bytes (for display,
// provenance, and patch generation) plus the string lists that declarative
// rules may reference, resolved by the domain adapter.
type PolicyDoc struct {
	Version int
	Raw     []byte
	lists   map[string][]string
}

// NewPolicyDoc wraps raw policy bytes and the list lookups an adapter
// resolved from them. Refs are dot paths, e.g. "preflop.premium".
func NewPolicyDoc(version int, raw []byte, lists map[string][]string) *PolicyDoc {
	return &PolicyDoc{Version: version, Raw: raw, lists: lists}
}

// List returns the string list at ref, if the adapter resolved it.
func (p *PolicyDoc) List(ref string) ([]string, bool) {
	l, ok := p.lists[ref]
	return l, ok
}

// Rule is one declarative acceptance criterion, loaded from the agent's
// rules file (one JSON file per agent, e.g. data/poker-rules.json). A
// decision violates the rule when it was made in Phase, took an action in
// Actions, and its ContextCode is a member of the policy list at ListRef.
// Empty Phase / Actions match anything.
type Rule struct {
	Name    string   `json:"name"`
	Phase   string   `json:"phase"`
	Actions []string `json:"actions"`
	ListRef string   `json:"list_ref"`
	Detail  string   `json:"detail"` // template; {{code}} and {{action}} are substituted
}

// LoadRules reads a declarative rule set from JSON and validates it.
func LoadRules(path string) ([]Rule, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rules []Rule
	if err := json.Unmarshal(raw, &rules); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for i, r := range rules {
		if r.Name == "" {
			return nil, fmt.Errorf("%s: rule %d has no name", path, i)
		}
		if r.ListRef == "" {
			return nil, fmt.Errorf("%s: rule %s has no list_ref", path, r.Name)
		}
	}
	return rules, nil
}

// Violation is one decision that broke one rule.
type Violation struct {
	Rule        string
	Phase       string
	ContextCode string
	Action      string
	Pot         float64
	Detail      string
}

// Report is the result of running a rule set against a set of decisions.
type Report struct {
	TotalDecisions int
	Violations     []Violation
	RulesRun       []string
}

// RunRules checks every decision against every rule and collects violations.
func RunRules(rules []Rule, policy *PolicyDoc, decisions []Decision) Report {
	r := Report{TotalDecisions: len(decisions)}
	for _, rule := range rules {
		r.RulesRun = append(r.RulesRun, rule.Name)
	}
	for _, d := range decisions {
		for _, rule := range rules {
			if !matches(rule, d, policy) {
				continue
			}
			detail := strings.ReplaceAll(rule.Detail, "{{code}}", d.ContextCode)
			detail = strings.ReplaceAll(detail, "{{action}}", d.Action)
			r.Violations = append(r.Violations, Violation{
				Rule:        rule.Name,
				Phase:       d.Phase,
				ContextCode: d.ContextCode,
				Action:      d.Action,
				Pot:         d.Pot,
				Detail:      detail,
			})
		}
	}
	return r
}

func matches(rule Rule, d Decision, p *PolicyDoc) bool {
	if rule.Phase != "" && rule.Phase != d.Phase {
		return false
	}
	if len(rule.Actions) > 0 && !containsFold(rule.Actions, d.Action) {
		return false
	}
	list, ok := p.List(rule.ListRef)
	if !ok {
		return false
	}
	return containsFold(list, d.ContextCode)
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
