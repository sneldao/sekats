// Package summarizer adapts a synthetic document-summary agent. Its fixture
// carries source inputs, the chosen action, rationale, outcome, and a linked
// shadow review for each decision; aggregate quality numbers are derived.
package summarizer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sneldao/sekats/internal/engine"
	"github.com/sneldao/sekats/internal/store"
)

const (
	Name           = "summarizer"
	PolicyFileName = "summarizer-policy.json"
	RulesFileName  = "summarizer-rules.json"
	FixtureFile    = "summarizer-decisions.json"
	PolicyTarget   = "review.promotedFromDecisionReviews"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (*Adapter) Name() string         { return Name }
func (*Adapter) Agent() string        { return "Mock document summarizer (synthetic fixture)" }
func (*Adapter) PolicyFile() string   { return PolicyFileName }
func (*Adapter) RulesFile() string    { return RulesFileName }
func (*Adapter) PolicyTarget() string { return PolicyTarget }
func (*Adapter) Labels() store.Labels {
	return store.Labels{Context: "Input class", Phase: "Stage"}
}

func (*Adapter) LoadPolicy(path string) (*engine.PolicyDoc, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Version int `json:"version"`
		Review  struct {
			RequireRevision []string `json:"requireRevision"`
			RequireWithhold []string `json:"requireWithhold"`
		} `json:"review"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return engine.NewPolicyDoc(doc.Version, raw, map[string][]string{
		"review.requireRevision": doc.Review.RequireRevision,
		"review.requireWithhold": doc.Review.RequireWithhold,
	}), nil
}

type fixtureDecision struct {
	store.Decision
	ReviewAction    string `json:"review_action"`
	ReviewRationale string `json:"review_rationale"`
}

// LoadFixtures is the only code translating this domain's fixture to the
// console store. No poker fields or precomputed quality_stats.json are needed.
func (*Adapter) LoadFixtures(dataDir string) (*store.Store, error) {
	path := filepath.Join(dataDir, FixtureFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var fixture struct {
		Synthetic   bool                     `json:"synthetic"`
		Decisions   []fixtureDecision        `json:"decisions"`
		Suggestions []store.PolicySuggestion `json:"policy_suggestions"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if !fixture.Synthetic || len(fixture.Decisions) == 0 {
		return nil, fmt.Errorf("%s: expected a non-empty synthetic fixture", path)
	}
	s := &store.Store{Synthetic: true}
	s.Decisions.PolicySuggestions = fixture.Suggestions
	q := &s.Quality
	q.PolicySuggestionsTotal = len(fixture.Suggestions)
	for _, suggestion := range fixture.Suggestions {
		if suggestion.Status == "promoted" {
			q.PolicySuggestionsPromoted++
		}
	}
	seen := make(map[int64]bool)
	for _, row := range fixture.Decisions {
		d := row.Decision
		if d.ID <= 0 || seen[d.ID] {
			return nil, fmt.Errorf("%s: invalid or duplicate decision ID %d", path, d.ID)
		}
		seen[d.ID] = true
		if d.Phase == "" || d.ContextCode == "" || d.Action == "" ||
			len(d.Inputs) == 0 || string(d.Inputs) == "null" ||
			d.Rationale == "" || d.Outcome == "" ||
			row.ReviewAction == "" || row.ReviewRationale == "" {
			return nil, fmt.Errorf("%s: incomplete decision %d", path, d.ID)
		}
		s.Decisions.RecentDecisions = append(s.Decisions.RecentDecisions, d)
		q.LLMReviewsTotal++
		q.LinkedReviews++
		q.AgreementRate.Total++
		found := false
		for i := range q.AgreementBreakdown {
			p := &q.AgreementBreakdown[i]
			if p.OurAction == d.Action && p.LLMAction == row.ReviewAction {
				p.Count++
				found = true
				break
			}
		}
		if !found {
			q.AgreementBreakdown = append(q.AgreementBreakdown, store.AgreementPair{
				OurAction: d.Action, LLMAction: row.ReviewAction, Count: 1,
			})
		}
		if d.Action == row.ReviewAction {
			q.AgreementRate.Agree++
			continue
		}
		response, err := json.Marshal(map[string]string{
			"action": row.ReviewAction, "rationale": row.ReviewRationale,
		})
		if err != nil {
			return nil, err
		}
		q.DisagreementsSample = append(q.DisagreementsSample, store.Disagreement{
			DecisionID: d.ID, Phase: d.Phase, ContextCode: d.ContextCode,
			Inputs: d.Inputs, AgentRationale: d.Rationale, Outcome: d.Outcome,
			OurAction: d.Action, LLMAction: row.ReviewAction, ResponseText: string(response),
		})
	}
	q.LLMReviewsByError = []store.ErrorBucket{{Bucket: "clean", Count: q.LLMReviewsTotal}}
	return s, nil
}

func (*Adapter) PatternKey(r engine.Review) string {
	return strings.ToLower(r.Phase + "/" + r.ContextCode + "/" + r.OurAction + "-to-" + r.LLMAction)
}

func (*Adapter) NewDraft(key string, ev []engine.Evidence) engine.Draft {
	first := ev[0]
	pattern := fmt.Sprintf("%s: %s to %s rejected by a human on %d summaries",
		first.ContextCode, first.OurAction, first.LLMAction, len(ev))
	rule := fmt.Sprintf("For %s inputs, %s rather than %s. Publish only after source-supported claims and attribution have been verified.",
		first.ContextCode, first.LLMAction, first.OurAction)
	if first.LLMAction == "withhold" {
		rule = "Withhold summaries containing sensitive records until an authorized reviewer has approved redaction and release."
	}
	// Keys contain only the fixture's controlled stage, category and actions.
	id := strings.NewReplacer("/", "-", "_", "-", " ", "-").Replace(key)
	return engine.Draft{
		ID: id, PatternKey: key, Pattern: pattern, Rule: rule, Target: PolicyTarget,
		Payload:  map[string]any{"inputClass": first.ContextCode, "pattern": pattern, "rule": rule},
		Evidence: append([]engine.Evidence{}, ev...), Status: engine.DraftPending,
		CreatedAt: time.Now().UTC(),
	}
}
