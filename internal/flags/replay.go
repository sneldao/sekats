package flags

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sneldao/sekats/internal/engine"
)

// ReplayInput is the adapter-neutral evidence needed to shadow one historical
// decision under the original or candidate action. CorrectAction is the
// human-adjudicated action; empty means the row is unlabelled and must not be
// counted as a conversion either way.
type ReplayInput struct {
	DecisionID      int64
	Phase           string
	ContextCode     string
	Pot             float64
	OriginalAction  string
	CandidateAction string
	CorrectAction   string
}

// RunReplay evaluates stable decision identities in Flagsmith, applies only
// the assigned variant's action to immutable history, and sends exposures plus
// human-verdict alignment and violation events. A failed or unenrolled remote evaluation never fabricates
// candidate behaviour: it falls back to control and records why.
func (c *Client) RunReplay(ctx context.Context, r Rollout, inputs []ReplayInput, policy *engine.PolicyDoc, rules []engine.Rule) (ReplaySummary, []ReplayRow, error) {
	summary := emptySummary()
	rows := make([]ReplayRow, 0, len(inputs))
	events := make([]Event, 0, len(inputs)*3)
	now := time.Now().UnixMilli()
	exposures := 0

	for _, input := range inputs {
		identifier := identityFor(input.DecisionID)
		evaluation, err := c.Evaluate(ctx, r.FlagName, identifier)
		row := ReplayRow{
			DecisionID: input.DecisionID,
			Identifier: identifier,
			Variant:    Control,
			Source:     "remote",
		}
		if err != nil {
			row.Source = "control_fallback"
			row.Error = err.Error()
			summary.Errors++
		} else {
			row.Variant = evaluation.Variant
			row.Enrolled = evaluation.InExperiment && evaluation.Enabled
			row.ExperimentID = evaluation.ExperimentID
			switch {
			case !evaluation.Enabled:
				row.Source = "flag_disabled"
				row.Variant = Control
			case !evaluation.InExperiment:
				row.Source = "unenrolled"
				row.Variant = Control
			}
		}

		action := input.OriginalAction
		if row.Enrolled && row.Variant == Candidate {
			action = input.CandidateAction
		}
		row.ShadowAction = action
		row.Agree = input.CorrectAction != "" && strings.EqualFold(strings.TrimSpace(action), strings.TrimSpace(input.CorrectAction))
		decision := engine.Decision{
			ID: input.DecisionID, Phase: input.Phase, ContextCode: input.ContextCode,
			Action: action, Pot: input.Pot,
		}
		report := engine.RunRules(rules, policy, []engine.Decision{decision})
		row.Violations = len(report.Violations)
		rows = append(rows, row)
		summary.Replayed++

		if !row.Enrolled {
			continue
		}
		variant := row.Variant
		if variant != Candidate {
			variant = Control
			row.Variant = Control
		}
		summary.Enrolled++
		summary.Total[variant]++
		if row.Agree {
			summary.Agree[variant]++
		}
		summary.Violations[variant] += row.Violations

		metadata := map[string]any{
			"experiment_id": r.ExperimentID, "draft_id": r.DraftID,
			"decision_id": input.DecisionID, "source": "shadow_replay",
		}
		traits := map[string]any{"phase": input.Phase, "context": input.ContextCode}
		events = append(events, Event{
			Event: "$flag_exposure", FeatureName: r.FlagName, Identifier: identifier,
			Value: variant, Traits: traits, Metadata: metadata, Timestamp: now,
		})
		exposures++
		if input.CorrectAction != "" {
			alignment := "0"
			if row.Agree {
				alignment = "1"
			}
			events = append(events, Event{
				Event: AgreementEvent, FeatureName: r.FlagName, Identifier: identifier,
				Value: alignment, Traits: traits, Metadata: metadata, Timestamp: now,
			})
		}
		if row.Violations > 0 {
			events = append(events, Event{
				Event: ViolationEvent, FeatureName: r.FlagName, Identifier: identifier,
				Value: strconv.Itoa(row.Violations), Traits: traits, Metadata: metadata, Timestamp: now,
			})
		}
	}

	sendErr := c.SendEvents(ctx, events)
	if sendErr == nil {
		summary.ExposureSent = exposures
	}
	summary.LastReplayAt = time.Now().UTC()
	if sendErr != nil {
		return summary, rows, fmt.Errorf("send experiment events: %w", sendErr)
	}
	return summary, rows, nil
}
