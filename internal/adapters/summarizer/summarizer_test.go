package summarizer_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sneldao/sekats/internal/adapters"
	"github.com/sneldao/sekats/internal/adapters/summarizer"
	"github.com/sneldao/sekats/internal/engine"
	"github.com/sneldao/sekats/internal/store"
)

var _ adapters.Adapter = summarizer.New()

func TestFixtureAndPolicy(t *testing.T) {
	ad := summarizer.New()
	s, err := ad.LoadFixtures("../../../data")
	if err != nil {
		t.Fatal(err)
	}
	if !s.Synthetic || len(s.Decisions.RecentDecisions) != 60 ||
		len(s.Quality.DisagreementsSample) != 36 ||
		s.Quality.AgreementRate.Agree != 24 || s.Quality.LinkedReviews != 60 {
		t.Fatalf("unexpected fixture counts: %+v", s.Quality)
	}
	seen := make(map[int64]store.Decision)
	for _, d := range s.Decisions.RecentDecisions {
		if d.ID <= 0 || seen[d.ID].ID != 0 || d.Street != "" || d.HandCode != "" ||
			len(d.Inputs) == 0 || d.Rationale == "" || d.Outcome == "" {
			t.Fatalf("incomplete or poker-shaped decision: %+v", d)
		}
		seen[d.ID] = d
	}
	for _, row := range s.Quality.DisagreementsSample {
		d := seen[row.DecisionID]
		if d.ID == 0 || row.OurAction != d.Action || row.Outcome != d.Outcome ||
			engine.ExtractRationale(row.ResponseText) == "" {
			t.Fatalf("review not linked to full decision evidence: %+v", row)
		}
	}
	pol, err := ad.LoadPolicy(filepath.Join("../../../data", ad.PolicyFile()))
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"review.requireRevision", "review.requireWithhold"} {
		list, ok := pol.List(ref)
		if !ok || len(list) == 0 {
			t.Fatalf("unresolved policy list %s", ref)
		}
	}
	if _, supportsLive := any(ad).(adapters.LiveAdapter); supportsLive {
		t.Fatal("synthetic summarizer must not load a live DB")
	}
}

func TestInvalidFixtures(t *testing.T) {
	ad := summarizer.New()
	raw, err := os.ReadFile("../../../data/" + summarizer.FixtureFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"duplicate IDs", func(f map[string]any) {
			ds := f["decisions"].([]any)
			ds[1].(map[string]any)["id"] = ds[0].(map[string]any)["id"]
		}},
		{"missing inputs", func(f map[string]any) {
			delete(f["decisions"].([]any)[0].(map[string]any), "inputs")
		}},
		{"null inputs", func(f map[string]any) {
			f["decisions"].([]any)[0].(map[string]any)["inputs"] = nil
		}},
		{"missing outcome", func(f map[string]any) {
			delete(f["decisions"].([]any)[0].(map[string]any), "outcome")
		}},
		{"missing review", func(f map[string]any) {
			delete(f["decisions"].([]any)[0].(map[string]any), "review_action")
		}},
		{"unlabelled mock", func(f map[string]any) { f["synthetic"] = false }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var fixture map[string]any
			if err := json.Unmarshal(raw, &fixture); err != nil {
				t.Fatal(err)
			}
			test.edit(fixture)
			changed, err := json.Marshal(fixture)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, summarizer.FixtureFile), changed, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := ad.LoadFixtures(dir); err == nil {
				t.Fatal("invalid fixture was accepted")
			}
		})
	}
}

func TestPatternClassesAndDraft(t *testing.T) {
	ad := summarizer.New()
	review := engine.Review{Phase: "Editorial", ContextCode: "sensitive_record", OurAction: "publish", LLMAction: "withhold"}
	key := ad.PatternKey(review)
	other := review
	other.ContextCode = "missing_attribution"
	if key == ad.PatternKey(other) {
		t.Fatal("different input risks share a pattern bucket")
	}
	ev := []engine.Evidence{{DecisionID: 1, Phase: review.Phase, ContextCode: review.ContextCode,
		OurAction: review.OurAction, LLMAction: review.LLMAction}}
	draft := ad.NewDraft(key, ev)
	ev[0].DecisionID = 99
	if draft.Evidence[0].DecisionID != 1 || draft.Target != ad.PolicyTarget() ||
		!strings.Contains(draft.Rule, "authorized reviewer") || strings.Contains(draft.ID, "/") {
		t.Fatalf("invalid draft: %+v", draft)
	}
}
