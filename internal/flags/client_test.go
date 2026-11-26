package flags

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sneldao/sekats/internal/engine"
)

func TestProvisionExperimentCreatesAndStarts(t *testing.T) {
	var paths []string
	var experimentBody map[string]any
	metricID := 100
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/warehouse-connections/") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode([]warehouse{})
		case strings.HasSuffix(r.URL.Path, "/warehouse-connections/") && r.Method == http.MethodPost:
			json.NewEncoder(w).Encode(warehouse{ID: 9, WarehouseType: "flagsmith", Status: "active"})
		case strings.Contains(r.URL.Path, "/features/") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{"count": 0, "results": []Feature{}})
		case strings.HasSuffix(r.URL.Path, "/features/") && r.Method == http.MethodPost:
			json.NewEncoder(w).Encode(Feature{ID: 11, Name: FlagName("draft-1"), InitialValue: Control, DefaultEnabled: true})
		case strings.Contains(r.URL.Path, "/mv-options/"):
			json.NewEncoder(w).Encode(MVOption{ID: 22, Key: Candidate, StringValue: Candidate, Feature: 11})
		case strings.Contains(r.URL.Path, "/experiment-metrics/") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{"count": 0, "results": []Metric{}})
		case strings.Contains(r.URL.Path, "/experiment-metrics/") && r.Method == http.MethodPost:
			metricID++
			json.NewEncoder(w).Encode(Metric{ID: metricID})
		case strings.Contains(r.URL.Path, "/experiments/") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{"count": 0, "results": []Experiment{}})
		case strings.Contains(r.URL.Path, "/experiments/") && r.Method == http.MethodPost:
			json.NewDecoder(r.Body).Decode(&experimentBody)
			json.NewEncoder(w).Encode(Experiment{ID: 44, Name: "sekats-draft-1", Status: "created"})
		case strings.HasSuffix(r.URL.Path, "/experiments/44/start/"):
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient(Config{APIURL: server.URL, EventsURL: server.URL, ProjectID: 7, EnvironmentKey: "env", ServerEnvironmentKey: "ser.test", ManagementToken: "secret", RolloutPercent: 20, CandidatePercent: 50})
	provisioned, err := client.ProvisionExperiment(context.Background(), engine.Draft{ID: "draft-1", Rule: "prefer the safer action"})
	if err != nil {
		t.Fatal(err)
	}
	if provisioned.Feature.ID != 11 || provisioned.Variant.ID != 22 || provisioned.Experiment.ID != 44 || provisioned.Experiment.Status != "running" {
		t.Fatalf("unexpected provisioning result: %+v", provisioned)
	}
	if provisioned.AgreementMetricID != 101 || provisioned.ViolationMetricID != 102 {
		t.Fatalf("metric ids not retained: %+v", provisioned)
	}
	joined := strings.Join(paths, "\n")
	if !strings.Contains(joined, "POST /api/v1/environments/env/experiments/44/start/") {
		t.Fatalf("experiment was not started:\n%s", joined)
	}
	rollout := experimentBody["experiment_rollout"].(map[string]any)
	if rollout["rollout_percentage"] != 20.0 {
		t.Fatalf("wrong rollout percentage in body: %#v", experimentBody)
	}
	mv := rollout["multivariate_feature_state_values"].([]any)[0].(map[string]any)
	if mv["multivariate_feature_option"] != 22.0 || mv["percentage_allocation"] != 50.0 {
		t.Fatalf("wrong variant allocation: %#v", mv)
	}
}

func TestRunReplayEvaluationFailureFallsBackToControl(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	client := NewClient(Config{APIURL: server.URL, EventsURL: server.URL, EnvironmentKey: "env"})
	policyDoc := engine.NewPolicyDoc(1, nil, map[string][]string{"risky": {"A7o"}})
	rules := []engine.Rule{{Name: "no risky raises", Actions: []string{"raise"}, ListRef: "risky"}}
	rollout := Rollout{DraftID: "draft-1", FlagName: "flag", ExperimentID: 44}
	inputs := []ReplayInput{{DecisionID: 42, ContextCode: "A7o", OriginalAction: "call", CandidateAction: "raise"}}
	summary, rows, err := client.RunReplay(context.Background(), rollout, inputs, policyDoc, rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Variant != Control || rows[0].Enrolled || rows[0].Source != "control_fallback" || rows[0].Error == "" {
		t.Fatalf("evaluation failure did not fall back to explicit control: %+v", rows)
	}
	if summary.Enrolled != 0 || summary.Errors != 1 {
		t.Fatalf("bad fallback summary: %+v", summary)
	}
}

func TestEventMarshalMatchesSDKWireShape(t *testing.T) {
	raw, err := json.Marshal(Event{Event: AgreementEvent, Identifier: "decision-42", Timestamp: 123})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"feature_name":null`, `"value":null`, `"traits":null`, `"identifier":"decision-42"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("event wire shape missing %s: %s", want, raw)
		}
	}
}

func TestStorePersistsRollout(t *testing.T) {
	path := t.TempDir() + "/rollouts.json"
	store := NewStore(path)
	in := Rollout{DraftID: "draft-1", FlagName: "flag", Status: StatusRunning, ExperimentID: 44}
	if err := store.Put(in); err != nil {
		t.Fatal(err)
	}
	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	out, ok := reloaded.Get("draft-1")
	if !ok || out.ExperimentID != 44 || out.CreatedAt.IsZero() || out.UpdatedAt.IsZero() {
		t.Fatalf("rollout not persisted: %+v", out)
	}
}

func TestEvaluateUsesSeparateEdgeEndpoint(t *testing.T) {
	var edgeHits, managementHits int
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		edgeHits++
		if got := r.Header.Get("X-Environment-Key"); got != "ser.test" {
			t.Errorf("evaluation used %q rather than the server-side environment key", got)
		}
		json.NewEncoder(w).Encode(map[string]any{"flags": []map[string]any{{
			"feature": map[string]any{"id": 11, "name": "flag"}, "enabled": true,
			"feature_state_value": Control,
			"metadata":            map[string]any{"experiment": map[string]any{"id": 44, "in_experiment": true}},
		}}})
	}))
	defer edge.Close()
	management := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		managementHits++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer management.Close()

	client := NewClient(Config{APIURL: management.URL, EvaluationURL: edge.URL, EnvironmentKey: "env", ServerEnvironmentKey: "ser.test"})
	evaluation, err := client.Evaluate(context.Background(), "flag", "decision-42")
	if err != nil {
		t.Fatal(err)
	}
	if !evaluation.InExperiment || evaluation.ExperimentID != 44 || edgeHits != 1 || managementHits != 0 {
		t.Fatalf("evaluation used wrong endpoint or metadata: %+v edge=%d management=%d", evaluation, edgeHits, managementHits)
	}
}

func TestResultsParsesWarehouseComputationPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/experiments/44/results/") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"results":{"as_of":"2026-11-25T12:00:00Z","last_error_at":null,"refresh_requested_at":"2026-11-25T11:59:00Z","is_final":false,"payload":{"srm_p_value":0.42,"metrics":[{"metric_id":101,"variants":{"control":{"n":20,"sum":8,"sum_squares":8},"candidate":{"n":22,"sum":15,"sum_squares":15}},"inference":{"candidate":{"lift":0.7,"ci_low":0.1,"ci_high":1.2,"chance_to_win":0.732}},"conversions_timeseries":null}],"exposures_timeseries":{"granularity":"day","points":[]}}}}`)
	}))
	defer server.Close()

	client := NewClient(Config{APIURL: server.URL, EnvironmentKey: "env", ManagementToken: "token"})
	results, err := client.Results(context.Background(), 44)
	if err != nil {
		t.Fatal(err)
	}
	if results == nil || results.SRMPValue != 0.42 || results.AsOf == "" || results.Final {
		t.Fatalf("bad results envelope: %+v", results)
	}
	chance, ok := results.CandidateChanceToWin(101)
	if !ok || chance != 0.732 {
		t.Fatalf("candidate chance not decoded: %v %v", chance, ok)
	}
}
