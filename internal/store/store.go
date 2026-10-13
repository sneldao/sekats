// Package store defines the shared console data shape. The original poker
// export fields remain readable for backwards compatibility; other adapters
// use Phase, ContextCode, Inputs, Rationale, and Outcome.
package store

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/sneldao/sekats/internal/engine"
)

// Labels is the display vocabulary supplied by a domain adapter.
// An empty Metric hides the legacy numeric value column.
type Labels struct {
	Context string
	Phase   string
	Metric  string
}

// Decision is one row from our_decisions, as produced by
// poker/tools/export_scenarios.py. The frozen export predates the id and
// decided_at columns here (they read zero in fixture mode); the live bridge
// (internal/live) populates them.
type Decision struct {
	Phase       string          `json:"phase,omitempty"`
	ContextCode string          `json:"context_code,omitempty"`
	Inputs      json.RawMessage `json:"inputs,omitempty"`
	Rationale   string          `json:"rationale,omitempty"`
	Outcome     string          `json:"outcome,omitempty"`
	ID          int64           `json:"id"`
	DecidedAt   int64           `json:"decided_at"`
	Mode        string          `json:"mode"`
	Street      string          `json:"street"`
	HandCode    string          `json:"hand_code"`
	BoardJSON   string          `json:"board_json"`
	Pot         float64         `json:"pot"`
	LegalJSON   string          `json:"legal_json"`
	Action      string          `json:"action"`
	Amount      *float64        `json:"amount"`
	Source      string          `json:"source"`
	Accepted    int             `json:"accepted"`
	Error       *string         `json:"error"`
}

// EngineDecision normalizes both the generic schema and legacy poker export.
func (d Decision) EngineDecision() engine.Decision {
	phase, code := d.Phase, d.ContextCode
	if phase == "" {
		phase = d.Street
	}
	if code == "" {
		code = d.HandCode
	}
	return engine.Decision{
		ID: d.ID, DecidedAt: d.DecidedAt, Phase: phase, ContextCode: code,
		Action: d.Action, Amount: d.Amount, Pot: d.Pot, Source: d.Source,
		Accepted: d.Accepted, Error: d.Error,
	}
}

// PolicySuggestion is one row from policy_suggestions. created_at is stored
// as an epoch-ms integer in SQLite (see AGENTS.md's warning about manual
// epoch-ms math elsewhere in this codebase) — kept as int64 here rather
// than converted, since this app only counts/displays suggestions and
// never needs wall-clock arithmetic on them.
type PolicySuggestion struct {
	CreatedAt      int64   `json:"created_at"`
	Model          string  `json:"model"`
	Summary        string  `json:"summary"`
	Status         string  `json:"status"`
	Error          *string `json:"error"`
	SuggestionJSON string  `json:"suggestion_json"`
}

// DecisionsExport mirrors the top-level shape of decisions_export.json.
type DecisionsExport struct {
	RecentDecisions   []Decision         `json:"recent_decisions"`
	PolicySuggestions []PolicySuggestion `json:"policy_suggestions"`
}

// ErrorBucket counts llm_reviews rows grouped by error string ("clean" for
// no error).
type ErrorBucket struct {
	Bucket string `json:"bucket"`
	Count  int    `json:"c"`
}

// AgreementPair counts how often our_decisions.action lined up (or not)
// with the shadow-LLM's suggested action.
type AgreementPair struct {
	OurAction string `json:"our_action"`
	LLMAction string `json:"llm_action"`
	Count     int    `json:"c"`
}

// AgreementRate is the overall agree/total count across linked reviews.
type AgreementRate struct {
	Agree int `json:"agree"`
	Total int `json:"total"`
}

// PreflopActionCount counts preflop actions by hand code.
type PreflopActionCount struct {
	HandCode string `json:"hand_code"`
	Action   string `json:"action"`
	Count    int    `json:"c"`
}

// Disagreement is one real hand where the agent and the shadow-LLM review
// disagreed on the action.
type Disagreement struct {
	Phase          string          `json:"phase,omitempty"`
	ContextCode    string          `json:"context_code,omitempty"`
	Inputs         json.RawMessage `json:"inputs,omitempty"`
	AgentRationale string          `json:"agent_rationale,omitempty"`
	Outcome        string          `json:"outcome,omitempty"`
	OurAction      string          `json:"our_action"`
	LLMAction      string          `json:"llm_action"`
	HandCode       string          `json:"hand_code"`
	Street         string          `json:"street"`
	Pot            float64         `json:"pot"`
	ResponseText   string          `json:"response_text"`
	DecisionID     int64           `json:"decision_id"`
}

// Review normalizes the context fields without requiring poker-shaped fixtures.
func (d Disagreement) Review() engine.Review {
	phase, code := d.Phase, d.ContextCode
	if phase == "" {
		phase = d.Street
	}
	if code == "" {
		code = d.HandCode
	}
	return engine.Review{
		DecisionID: d.DecisionID, Phase: phase, ContextCode: code,
		OurAction: d.OurAction, LLMAction: d.LLMAction, Pot: d.Pot,
		ResponseText: d.ResponseText,
	}
}

// QualityStats mirrors the top-level shape of quality_stats.json.
type QualityStats struct {
	PolicySuggestionsTotal    int                  `json:"policy_suggestions_total"`
	PolicySuggestionsPromoted int                  `json:"policy_suggestions_promoted"`
	LLMReviewsTotal           int                  `json:"llm_reviews_total"`
	LLMReviewsByError         []ErrorBucket        `json:"llm_reviews_by_error"`
	LinkedReviews             int                  `json:"linked_reviews"`
	AgreementBreakdown        []AgreementPair      `json:"agreement_breakdown"`
	AgreementRate             AgreementRate        `json:"agreement_rate"`
	PreflopActionDist         []PreflopActionCount `json:"preflop_action_dist"`
	DisagreementsSample       []Disagreement       `json:"disagreements_sample"`
}

// Policy mirrors the fields of poker-policy.json this app needs. Unknown
// fields are ignored via json.RawMessage passthrough where relevant, but for
// the AC engine we only need the preflop hand-list sections.
type Policy struct {
	Version int `json:"version"`
	Preflop struct {
		Premium                 []string `json:"premium"`
		Strong                  []string `json:"strong"`
		PlayableSuited          []string `json:"playableSuited"`
		PlayableOffsuitLate     []string `json:"playableOffsuitLate"`
		DominatedFoldCandidates []string `json:"dominatedFoldCandidates"`
		TrashExamples           []string `json:"trashExamples"`
	} `json:"preflop"`
}

// Store holds all loaded fixture data for the app's lifetime. It is
// read-only after Load — the review queue's mutable state lives separately
// in internal/review.
type Store struct {
	Synthetic bool // explicitly labelled in the source badge
	Decisions DecisionsExport
	Quality   QualityStats
	Policy    Policy
}

// Load reads all fixture JSON files from dataDir (typically "data/").
func Load(dataDir string) (*Store, error) {
	s := &Store{}

	if err := readJSON(filepath.Join(dataDir, "decisions_export.json"), &s.Decisions); err != nil {
		return nil, err
	}
	if err := readJSON(filepath.Join(dataDir, "quality_stats.json"), &s.Quality); err != nil {
		return nil, err
	}
	if err := readJSON(filepath.Join(dataDir, "poker-policy.json"), &s.Policy); err != nil {
		return nil, err
	}

	return s, nil
}

func readJSON(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(v)
}
