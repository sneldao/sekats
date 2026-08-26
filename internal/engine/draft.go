package engine

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

// Evidence is one human-rejected disagreement backing a policy draft.
type Evidence struct {
	DecisionID  int64     `json:"decision_id"`
	Phase       string    `json:"phase"`
	ContextCode string    `json:"context_code"`
	OurAction   string    `json:"our_action"`
	LLMAction   string    `json:"llm_action"`
	Pot         float64   `json:"pot"`
	Rationale   string    `json:"rationale"`
	RecordedAt  time.Time `json:"recorded_at"`
}

// Draft statuses.
const (
	DraftPending   = "pending"
	DraftApproved  = "approved"
	DraftDiscarded = "discarded"
)

// Draft is a policy edit auto-drafted when a human keeps rejecting the
// agent on a repeated pattern. It is staged for human review, not applied:
// approving it yields a Patch artifact that a human applies to the agent's
// policy file. The target section and entry shape are adapter-specific
// (for poker: an entry in postflop.promotedFromHandReviews, the same
// section the agent's existing promotions live in).
type Draft struct {
	ID         string         `json:"id"`
	PatternKey string         `json:"pattern_key"`
	Pattern    string         `json:"pattern"` // human description of the repeated spot
	Rule       string         `json:"rule"`    // the policy line to add
	Target     string         `json:"target"`  // dot path into the policy doc
	Payload    map[string]any `json:"entry"`   // the exact object to append
	Evidence   []Evidence     `json:"evidence"`
	Status     string         `json:"status"`
	CreatedAt  time.Time      `json:"created_at"`
	ResolvedAt *time.Time     `json:"resolved_at"`
}

// Patch is the machine-applicable artifact produced when a draft is
// approved: exactly what to append where, plus provenance. See
// tools/apply_policy_draft.py for the consumer.
type Patch struct {
	Schema          string         `json:"schema"`
	DraftID         string         `json:"draft_id"`
	PatternKey      string         `json:"pattern_key"`
	Target          string         `json:"target"`
	Entry           map[string]any `json:"entry"`
	SourceDecisions []int64        `json:"source_decisions"`
	GeneratedAt     time.Time      `json:"generated_at"`
}

// Patch renders the approved draft as its applyable artifact.
func (d *Draft) Patch() Patch {
	ids := make([]int64, 0, len(d.Evidence))
	for _, e := range d.Evidence {
		ids = append(ids, e.DecisionID)
	}
	return Patch{
		Schema:          "sekats-policy-patch/v1",
		DraftID:         d.ID,
		PatternKey:      d.PatternKey,
		Target:          d.Target,
		Entry:           d.Payload,
		SourceDecisions: ids,
		GeneratedAt:     time.Now().UTC(),
	}
}

// ExtractRationale pulls the "rationale" string out of a shadow-review
// response (a JSON snippet like {"action": "...", "rationale": "..."}).
// Returns "" when the field is absent or unparseable — evidence may simply
// carry no rationale, and the console must not fabricate one.
func ExtractRationale(responseText string) string {
	m := rationaleRe.FindStringSubmatch(strings.TrimSpace(responseText))
	if len(m) < 2 {
		return ""
	}
	var s string
	if err := json.Unmarshal([]byte(`"`+m[1]+`"`), &s); err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

var rationaleRe = regexp.MustCompile(`"rationale"\s*:\s*"((?:[^"\\]|\\.)*)"`)
