// Package trends computes the display-ready series for the Trends screen.
//
// One honesty constraint shapes what this screen can claim: the frozen
// fixture does not carry per-review timestamps, so an agreement-rate-over-
// time series cannot be built without fabricating data. Instead the screen
// shows what the fixture actually contains: the shadow-LLM policy-suggestion
// activity timeline (real epoch-ms created_at values), the disagreement
// pattern distribution, and the current snapshot figures.

package trends

import (
	"fmt"
	"sort"
	"time"

	"github.com/sneldao/sekats/internal/store"
)

// SuggestionEvent is one policy-suggestion row with its timestamp decoded.
type SuggestionEvent struct {
	Time       time.Time
	Status     string
	Model      string
	Summary    string
	Error      string
	SecondsAgo int64
}

// Pattern is one agent-action vs shadow-LLM-action pair with its count.
type Pattern struct {
	OurAction string
	LLMAction string
	Count     int
	Agree     bool
	PctOfAll  float64
}

// View is everything the /trends screen needs.
type View struct {
	Events     []SuggestionEvent
	Earliest   time.Time
	Latest     time.Time
	WindowText string
	ByStatus   map[string]int

	Patterns         []Pattern
	TopDisagreeCount int

	AgreementAgree        int
	AgreementTotal        int
	AgreementRatePct      float64
	LinkedReviews         int
	ReviewsWithTimestamps int
}

// Build derives a View from the loaded fixtures.
func Build(sugg []store.PolicySuggestion, q store.QualityStats) View {
	v := View{ByStatus: make(map[string]int)}

	for _, s := range sugg {
		status := s.Status
		if status == "" {
			status = "unknown"
		}
		errText := ""
		if s.Error != nil {
			errText = *s.Error
		}
		summary := s.Summary
		if summary == "" {
			summary = "(no summary — suggestion errored)"
		}
		v.Events = append(v.Events, SuggestionEvent{
			Time:    time.UnixMilli(s.CreatedAt).UTC(),
			Status:  status,
			Model:   s.Model,
			Summary: summary,
			Error:   errText,
		})
		v.ByStatus[status]++
	}

	sort.Slice(v.Events, func(i, j int) bool { return v.Events[i].Time.After(v.Events[j].Time) })
	if len(v.Events) > 0 {
		v.Earliest = v.Events[len(v.Events)-1].Time
		v.Latest = v.Events[0].Time
		span := v.Latest.Sub(v.Earliest)
		v.WindowText = fmt.Sprintf("%.0f min", span.Minutes())
		now := v.Latest // anchor "seconds ago" to the newest event, not wall-clock time
		for i := range v.Events {
			v.Events[i].SecondsAgo = int64(now.Sub(v.Events[i].Time).Seconds())
		}
	}

	for _, p := range q.AgreementBreakdown {
		v.Patterns = append(v.Patterns, Pattern{
			OurAction: p.OurAction,
			LLMAction: p.LLMAction,
			Count:     p.Count,
			Agree:     p.OurAction == p.LLMAction,
		})
	}
	total := 0
	for _, p := range v.Patterns {
		total += p.Count
	}
	maxDisagree := 0
	for i := range v.Patterns {
		v.Patterns[i].PctOfAll = 0
		if total > 0 {
			v.Patterns[i].PctOfAll = float64(v.Patterns[i].Count) / float64(total) * 100
		}
		if !v.Patterns[i].Agree && v.Patterns[i].Count > maxDisagree {
			maxDisagree = v.Patterns[i].Count
		}
	}
	v.TopDisagreeCount = maxDisagree

	sort.Slice(v.Patterns, func(i, j int) bool {
		if v.Patterns[i].Count != v.Patterns[j].Count {
			return v.Patterns[i].Count > v.Patterns[j].Count
		}
		return v.Patterns[i].OurAction < v.Patterns[j].OurAction
	})

	v.AgreementAgree = q.AgreementRate.Agree
	v.AgreementTotal = q.AgreementRate.Total
	v.LinkedReviews = q.LinkedReviews
	if v.AgreementTotal > 0 {
		v.AgreementRatePct = float64(v.AgreementAgree) / float64(v.AgreementTotal) * 100
	}
	v.ReviewsWithTimestamps = 0 // the fixture carries no per-review timestamps
	return v
}

// Bar is a template-friendly (label, percent-of-max) pair for the pattern
// distribution bars.
type Bar struct {
	Label string
	Value int
	Pct   float64 // of all linked reviews
	Agree bool
}

// PatternBars converts Patterns into bars scaled to the largest count.
func PatternBars(patterns []Pattern) []Bar {
	if len(patterns) == 0 {
		return nil
	}
	max := 0
	for _, p := range patterns {
		if p.Count > max {
			max = p.Count
		}
	}
	bars := make([]Bar, 0, len(patterns))
	for _, p := range patterns {
		kind := "agree"
		if !p.Agree {
			kind = "disagree"
		}
		bars = append(bars, Bar{
			Label: fmt.Sprintf("%s → %s (%s)", p.OurAction, p.LLMAction, kind),
			Value: p.Count,
			Pct:   p.PctOfAll,
			Agree: p.Agree,
		})
	}
	return bars
}
