// Package flags integrates the promotion pipeline with Flagsmith: management
// API provisioning, identity evaluation, experiment events, and results. It
// deliberately uses the plain REST API so the console has no SDK lifecycle or
// background event worker; the server flushes each small replay batch inline.
package flags

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sneldao/sekats/internal/engine"
)

const (
	Control        = "control"
	Candidate      = "candidate"
	AgreementEvent = "sekats_human_verdict_alignment"
	ViolationEvent = "sekats_policy_violation"
)

// Config is the operator-provided Flagsmith wiring. ManagementToken is a
// secret organisation API key and must never be rendered or committed.
type Config struct {
	APIURL               string
	EvaluationURL        string
	EventsURL            string
	DashboardURL         string
	ProjectID            int
	EnvironmentKey       string
	ServerEnvironmentKey string
	ManagementToken      string
	RolloutPercent       float64
	CandidatePercent     float64
}

func ConfigFromEnv() Config {
	project, _ := strconv.Atoi(os.Getenv("FLAGSMITH_PROJECT_ID"))
	rollout := percentEnv("FLAGSMITH_ROLLOUT_PERCENT", 20)
	candidate := percentEnv("FLAGSMITH_CANDIDATE_PERCENT", 50)
	apiURL := envOr("FLAGSMITH_API_URL", "https://api.flagsmith.com")
	evaluationFallback := "https://edge.api.flagsmith.com"
	if os.Getenv("FLAGSMITH_API_URL") != "" {
		// Self-hosted installations normally expose management and SDK evaluation
		// from the same API service. Cloud keeps the split edge endpoint instead.
		evaluationFallback = apiURL
	}
	return Config{
		APIURL:               apiURL,
		EvaluationURL:        envOr("FLAGSMITH_EVALUATION_API_URL", evaluationFallback),
		EventsURL:            envOr("FLAGSMITH_EVENTS_URL", "https://events.api.flagsmith.com"),
		DashboardURL:         envOr("FLAGSMITH_DASHBOARD_URL", "https://app.flagsmith.com"),
		ProjectID:            project,
		EnvironmentKey:       os.Getenv("FLAGSMITH_ENVIRONMENT_KEY"),
		ServerEnvironmentKey: os.Getenv("FLAGSMITH_SERVER_ENVIRONMENT_KEY"),
		ManagementToken:      firstEnv("FLAGSMITH_API_TOKEN", "FLAGSMITH_API_KEY"),
		RolloutPercent:       rollout,
		CandidatePercent:     candidate,
	}
}

func percentEnv(key string, fallback float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil && v > 0 && v <= 100 {
		return v
	}
	return fallback
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func (c Config) Configured() bool {
	return c.ManagementToken != "" && c.EnvironmentKey != "" && c.ServerEnvironmentKey != "" && c.ProjectID > 0
}

// Client is a small REST client for the exact API surface sekats needs.
type Client struct {
	cfg Config
	hc  *http.Client
}

func NewClient(cfg Config) *Client {
	cfg.APIURL = apiBaseURL(cfg.APIURL)
	if cfg.EvaluationURL == "" {
		cfg.EvaluationURL = cfg.APIURL
	}
	cfg.EvaluationURL = apiBaseURL(cfg.EvaluationURL)
	return &Client{cfg: cfg, hc: &http.Client{Timeout: 12 * time.Second}}
}

// apiBaseURL accepts either the service origin or the API-prefixed URL shown
// in Flagsmith docs, then stores the origin because this client appends
// /api/v1 itself.
func apiBaseURL(raw string) string {
	base := strings.TrimRight(raw, "/")
	base = strings.TrimSuffix(base, "/api/v1")
	return strings.TrimSuffix(base, "/api")
}

func (c *Client) Config() Config { return c.cfg }

func FlagName(draftID string) string {
	return "sekats_policy_" + strings.ReplaceAll(draftID, "-", "_")
}

func experimentName(draftID string) string { return "sekats-" + draftID }

func identityFor(decisionID int64) string { return fmt.Sprintf("decision-%d", decisionID) }

type apiError struct {
	status int
	body   string
}

func (e *apiError) Error() string { return fmt.Sprintf("flagsmith API %d: %s", e.status, e.body) }

func (c *Client) do(ctx context.Context, method, endpoint string, body, out any, management bool) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if management {
		req.Header.Set("Authorization", "Api-Key "+c.cfg.ManagementToken)
	} else {
		req.Header.Set("X-Environment-Key", c.cfg.ServerEnvironmentKey)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := strings.TrimSpace(string(raw))
		if len(snippet) > 500 {
			snippet = snippet[:500] + "…"
		}
		return &apiError{status: resp.StatusCode, body: snippet}
	}
	if out == nil || len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *Client) managementURL(path string) string {
	return strings.TrimRight(c.cfg.APIURL, "/") + path
}

func (c *Client) evaluationURL(path string) string {
	return strings.TrimRight(c.cfg.EvaluationURL, "/") + path
}

func (c *Client) eventsURL() string {
	base := strings.TrimRight(c.cfg.EventsURL, "/")
	if strings.HasSuffix(base, "/api/v1") {
		return base + "/events"
	}
	return base + "/v1/events"
}

type Feature struct {
	ID                  int        `json:"id"`
	Name                string     `json:"name"`
	InitialValue        string     `json:"initial_value"`
	DefaultEnabled      bool       `json:"default_enabled"`
	Description         string     `json:"description"`
	MultivariateOptions []MVOption `json:"multivariate_options"`
}

type MVOption struct {
	ID                          int     `json:"id"`
	Type                        string  `json:"type"`
	StringValue                 string  `json:"string_value"`
	DefaultPercentageAllocation float64 `json:"default_percentage_allocation"`
	Key                         string  `json:"key"`
	Feature                     int     `json:"feature"`
}

type Metric struct {
	ID          int             `json:"id"`
	Name        string          `json:"name"`
	Aggregation string          `json:"aggregation"`
	Direction   string          `json:"direction"`
	Definition  json.RawMessage `json:"definition"`
}

type Experiment struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type warehouse struct {
	ID            int    `json:"id"`
	WarehouseType string `json:"warehouse_type"`
	Status        string `json:"status"`
	StatusDetail  string `json:"status_detail"`
}

func paginated[T any](raw json.RawMessage) ([]T, error) {
	var page struct {
		Results []T `json:"results"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, err
	}
	return page.Results, nil
}

func (c *Client) getPaginated(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, c.managementURL(path), nil, out, true)
}

func (c *Client) EnsureWarehouse(ctx context.Context) error {
	path := fmt.Sprintf("/api/v1/environments/%s/warehouse-connections/", url.PathEscape(c.cfg.EnvironmentKey))
	var existing []warehouse
	if err := c.getPaginated(ctx, path, &existing); err != nil {
		return err
	}
	if len(existing) > 0 {
		for _, w := range existing {
			if w.Status != "errored" {
				return nil
			}
		}
		return fmt.Errorf("warehouse connection is errored: %s", existing[0].StatusDetail)
	}
	var created warehouse
	body := map[string]any{"warehouse_type": "flagsmith", "name": "Flagsmith", "config": map[string]any{}}
	if err := c.do(ctx, http.MethodPost, c.managementURL(path), body, &created, true); err != nil {
		return fmt.Errorf("enable managed warehouse: %w", err)
	}
	return nil
}

func (c *Client) findFeature(ctx context.Context, name string) (*Feature, error) {
	path := fmt.Sprintf("/api/v1/projects/%d/features/?search=%s&page_size=100", c.cfg.ProjectID, url.QueryEscape(name))
	var raw json.RawMessage
	if err := c.getPaginated(ctx, path, &raw); err != nil {
		return nil, err
	}
	features, err := paginated[Feature](raw)
	if err != nil {
		return nil, err
	}
	for _, f := range features {
		if f.Name == name {
			cp := f
			return &cp, nil
		}
	}
	return nil, nil
}

func (c *Client) createFeature(ctx context.Context, name, description string) (*Feature, error) {
	enabled := true
	value := Control
	body := map[string]any{
		"name": name, "description": description, "initial_value": value,
		"default_enabled": enabled, "is_server_key_only": true,
	}
	var f Feature
	path := fmt.Sprintf("/api/v1/projects/%d/features/", c.cfg.ProjectID)
	if err := c.do(ctx, http.MethodPost, c.managementURL(path), body, &f, true); err != nil {
		return nil, err
	}
	return &f, nil
}

func (c *Client) createVariant(ctx context.Context, featureID int) (*MVOption, error) {
	body := map[string]any{
		"feature": featureID, "type": "unicode", "string_value": Candidate,
		"key": Candidate, "default_percentage_allocation": 0,
	}
	var v MVOption
	path := fmt.Sprintf("/api/v1/projects/%d/features/%d/mv-options/", c.cfg.ProjectID, featureID)
	if err := c.do(ctx, http.MethodPost, c.managementURL(path), body, &v, true); err != nil {
		return nil, err
	}
	return &v, nil
}

func (c *Client) ensureFeature(ctx context.Context, draft engine.Draft) (*Feature, *MVOption, error) {
	name := FlagName(draft.ID)
	f, err := c.findFeature(ctx, name)
	if err != nil {
		return nil, nil, err
	}
	if f == nil {
		description := "Sekats governed rollout for policy draft " + draft.ID
		if f, err = c.createFeature(ctx, name, description); err != nil {
			return nil, nil, err
		}
	}
	for _, v := range f.MultivariateOptions {
		if v.Key == Candidate {
			cp := v
			return f, &cp, nil
		}
	}
	v, err := c.createVariant(ctx, f.ID)
	if err != nil {
		return nil, nil, err
	}
	return f, v, nil
}

func (c *Client) ensureMetric(ctx context.Context, name, event, aggregation, direction string) (*Metric, error) {
	path := fmt.Sprintf("/api/v1/environments/%s/experiment-metrics/?q=%s&page_size=100", url.PathEscape(c.cfg.EnvironmentKey), url.QueryEscape(name))
	var raw json.RawMessage
	if err := c.getPaginated(ctx, path, &raw); err != nil {
		return nil, err
	}
	metrics, err := paginated[Metric](raw)
	if err != nil {
		return nil, err
	}
	for _, m := range metrics {
		if m.Name == name {
			cp := m
			return &cp, nil
		}
	}
	body := map[string]any{
		"name": name, "aggregation": aggregation, "direction": direction,
		"definition": map[string]any{"version": 1, "event": event},
	}
	var m Metric
	createPath := fmt.Sprintf("/api/v1/environments/%s/experiment-metrics/", url.PathEscape(c.cfg.EnvironmentKey))
	if err := c.do(ctx, http.MethodPost, c.managementURL(createPath), body, &m, true); err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *Client) findExperiment(ctx context.Context, name string) (*Experiment, error) {
	path := fmt.Sprintf("/api/v1/environments/%s/experiments/?q=%s&page_size=100", url.PathEscape(c.cfg.EnvironmentKey), url.QueryEscape(name))
	var raw json.RawMessage
	if err := c.getPaginated(ctx, path, &raw); err != nil {
		return nil, err
	}
	experiments, err := paginated[Experiment](raw)
	if err != nil {
		return nil, err
	}
	for _, e := range experiments {
		if e.Name == name {
			cp := e
			return &cp, nil
		}
	}
	return nil, nil
}

func (c *Client) createExperiment(ctx context.Context, draft engine.Draft, featureID, variantID, agreementMetricID, violationMetricID int) (*Experiment, error) {
	name := experimentName(draft.ID)
	hypothesis := fmt.Sprintf("The candidate policy line (%s) will increase human-verdict alignment without increasing policy-as-AC violations compared with control.", draft.Rule)
	body := map[string]any{
		"feature": featureID, "name": name, "hypothesis": hypothesis,
		"metrics": []map[string]any{
			{"metric": agreementMetricID, "expected_direction": "increase"},
			{"metric": violationMetricID, "expected_direction": "decrease"},
		},
		"experiment_rollout": map[string]any{
			"enabled": true, "rollout_percentage": c.cfg.RolloutPercent,
			"feature_state_value": map[string]any{"type": "string", "value": Control},
			"multivariate_feature_state_values": []map[string]any{
				{"multivariate_feature_option": variantID, "percentage_allocation": c.cfg.CandidatePercent},
			},
		},
	}
	var e Experiment
	path := fmt.Sprintf("/api/v1/environments/%s/experiments/", url.PathEscape(c.cfg.EnvironmentKey))
	if err := c.do(ctx, http.MethodPost, c.managementURL(path), body, &e, true); err != nil {
		return nil, err
	}
	return &e, nil
}

func (c *Client) startExperiment(ctx context.Context, id int) error {
	path := fmt.Sprintf("/api/v1/environments/%s/experiments/%d/start/", url.PathEscape(c.cfg.EnvironmentKey), id)
	return c.do(ctx, http.MethodPost, c.managementURL(path), map[string]any{}, nil, true)
}

// ProvisionedExperiment is the complete remote experiment topology plus the
// metric ids needed to interpret the asymmetric results payload later.
type ProvisionedExperiment struct {
	Feature           *Feature
	Variant           *MVOption
	Experiment        *Experiment
	AgreementMetricID int
	ViolationMetricID int
}

// ProvisionExperiment idempotently creates the flag, keyed candidate variant,
// metrics, experiment, and starts it. It also enables the managed Flagsmith
// warehouse when the environment has none.
func (c *Client) ProvisionExperiment(ctx context.Context, draft engine.Draft) (*ProvisionedExperiment, error) {
	if !c.cfg.Configured() {
		return nil, fmt.Errorf("Flagsmith is not configured")
	}
	if err := c.EnsureWarehouse(ctx); err != nil {
		return nil, err
	}
	feature, variant, err := c.ensureFeature(ctx, draft)
	if err != nil {
		return nil, err
	}
	agreement, err := c.ensureMetric(ctx, "Sekats human-verdict alignment", AgreementEvent, "mean", "up")
	if err != nil {
		return nil, err
	}
	violations, err := c.ensureMetric(ctx, "Sekats policy violations", ViolationEvent, "sum", "down")
	if err != nil {
		return nil, err
	}
	name := experimentName(draft.ID)
	experiment, err := c.findExperiment(ctx, name)
	if err != nil {
		return nil, err
	}
	if experiment == nil {
		experiment, err = c.createExperiment(ctx, draft, feature.ID, variant.ID, agreement.ID, violations.ID)
		if err != nil {
			return nil, err
		}
	}
	status := strings.ToLower(experiment.Status)
	if status == "" || status == "created" || status == "draft" {
		if err := c.startExperiment(ctx, experiment.ID); err != nil {
			return nil, err
		}
		experiment.Status = "running"
	}
	return &ProvisionedExperiment{
		Feature: feature, Variant: variant, Experiment: experiment,
		AgreementMetricID: agreement.ID, ViolationMetricID: violations.ID,
	}, nil
}

// Evaluation is the identity-specific flag state needed by the replay harness.
type Evaluation struct {
	Variant      string
	Value        string
	Enabled      bool
	InExperiment bool
	ExperimentID int
	Reason       string
}

func (c *Client) Evaluate(ctx context.Context, flagName, identifier string) (Evaluation, error) {
	endpoint := c.evaluationURL("/api/v1/identities/?identifier=" + url.QueryEscape(identifier))
	var response struct {
		Flags []struct {
			Feature struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"feature"`
			Enabled           bool            `json:"enabled"`
			FeatureStateValue json.RawMessage `json:"feature_state_value"`
			Variant           *string         `json:"variant"`
			Reason            *string         `json:"reason"`
			Metadata          struct {
				Experiment *struct {
					ID           int    `json:"id"`
					Name         string `json:"name"`
					InExperiment bool   `json:"in_experiment"`
				} `json:"experiment"`
			} `json:"metadata"`
		} `json:"flags"`
	}
	if err := c.do(ctx, http.MethodGet, endpoint, nil, &response, false); err != nil {
		return Evaluation{}, err
	}
	for _, f := range response.Flags {
		if f.Feature.Name != flagName {
			continue
		}
		e := Evaluation{Variant: Control, Enabled: f.Enabled}
		if f.Variant != nil && *f.Variant != "" {
			e.Variant = *f.Variant
		}
		if f.Reason != nil {
			e.Reason = *f.Reason
		}
		var v any
		if len(f.FeatureStateValue) > 0 {
			_ = json.Unmarshal(f.FeatureStateValue, &v)
			e.Value = fmt.Sprint(v)
		}
		if f.Metadata.Experiment != nil {
			e.InExperiment = f.Metadata.Experiment.InExperiment
			e.ExperimentID = f.Metadata.Experiment.ID
		}
		return e, nil
	}
	return Evaluation{}, fmt.Errorf("flag %s not present in identity evaluation", flagName)
}

// Event is one experimentation event accepted by Flagsmith's ingestion API.
type Event struct {
	Event       string         `json:"event"`
	FeatureName string         `json:"feature_name,omitempty"`
	Identifier  string         `json:"identifier,omitempty"`
	Value       string         `json:"value,omitempty"`
	Traits      map[string]any `json:"traits,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Timestamp   int64          `json:"timestamp"`
}

// MarshalJSON mirrors the Flagsmith SDK wire shape: optional scalar fields
// are explicit nulls rather than omitted keys. Some ingestion validators are
// stricter about key presence than ordinary JSON APIs.
func (e Event) MarshalJSON() ([]byte, error) {
	nullable := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	return json.Marshal(struct {
		Event       string         `json:"event"`
		FeatureName *string        `json:"feature_name"`
		Identifier  *string        `json:"identifier"`
		Value       *string        `json:"value"`
		Traits      map[string]any `json:"traits"`
		Metadata    map[string]any `json:"metadata"`
		Timestamp   int64          `json:"timestamp"`
	}{
		Event: e.Event, FeatureName: nullable(e.FeatureName), Identifier: nullable(e.Identifier),
		Value: nullable(e.Value), Traits: e.Traits, Metadata: e.Metadata, Timestamp: e.Timestamp,
	})
}

func (c *Client) SendEvents(ctx context.Context, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	for i := range events {
		if events[i].Metadata == nil {
			events[i].Metadata = map[string]any{}
		}
		events[i].Metadata["sdk_version"] = "sekats-flagsmith-rest/v1"
	}
	var batch struct {
		Accepted int `json:"accepted"`
		Rejected []struct {
			Index int    `json:"index"`
			Error string `json:"error"`
		} `json:"rejected"`
	}
	if err := c.do(ctx, http.MethodPost, c.eventsURL(), map[string]any{"events": events}, &batch, false); err != nil {
		return err
	}
	if len(batch.Rejected) > 0 {
		first := batch.Rejected[0]
		return fmt.Errorf("flagsmith rejected %d experiment events (first index %d: %s)", len(batch.Rejected), first.Index, first.Error)
	}
	return nil
}

// Results is the payload inside Flagsmith's per-experiment results row. The
// beta API does not use the legacy trait-analytics shape: it returns aggregate
// sufficient statistics per metric plus inference against control.
type Results struct {
	SRMPValue float64        `json:"srm_p_value"`
	Metrics   []MetricResult `json:"metrics"`
	AsOf      string         `json:"-"`
	Final     bool           `json:"-"`
}

type MetricResult struct {
	MetricID  int                     `json:"metric_id"`
	Variants  map[string]VariantStats `json:"variants"`
	Inference map[string]*Inference   `json:"inference"`
}

type VariantStats struct {
	N          int     `json:"n"`
	Sum        float64 `json:"sum"`
	SumSquares float64 `json:"sum_squares"`
}

type Inference struct {
	Lift        float64 `json:"lift"`
	CILow       float64 `json:"ci_low"`
	CIHigh      float64 `json:"ci_high"`
	ChanceToWin float64 `json:"chance_to_win"`
}

// CandidateChanceToWin returns the candidate treatment's win probability for
// the requested metric. Inference is absent below Flagsmith's data floor.
func (r *Results) CandidateChanceToWin(metricID int) (float64, bool) {
	if r == nil {
		return 0, false
	}
	if metricID > 0 {
		for _, metric := range r.Metrics {
			if metric.MetricID != metricID {
				continue
			}
			if inference := metric.Inference[Candidate]; inference != nil {
				return inference.ChanceToWin, true
			}
			return 0, false
		}
	}
	for _, metric := range r.Metrics {
		if inference := metric.Inference[Candidate]; inference != nil {
			return inference.ChanceToWin, true
		}
	}
	return 0, false
}

func (c *Client) RefreshResults(ctx context.Context, experimentID int) error {
	path := fmt.Sprintf("/api/v1/environments/%s/experiments/%d/results/refresh/", url.PathEscape(c.cfg.EnvironmentKey), experimentID)
	return c.do(ctx, http.MethodPost, c.managementURL(path), map[string]any{}, nil, true)
}

func (c *Client) Results(ctx context.Context, experimentID int) (*Results, error) {
	path := fmt.Sprintf("/api/v1/environments/%s/experiments/%d/results/", url.PathEscape(c.cfg.EnvironmentKey), experimentID)
	var response struct {
		Results *struct {
			AsOf    string   `json:"as_of"`
			Payload *Results `json:"payload"`
			Final   bool     `json:"is_final"`
		} `json:"results"`
	}
	if err := c.do(ctx, http.MethodGet, c.managementURL(path), nil, &response, true); err != nil {
		return nil, err
	}
	if response.Results == nil || response.Results.Payload == nil {
		return nil, nil
	}
	results := response.Results.Payload
	results.AsOf = response.Results.AsOf
	results.Final = response.Results.Final
	return results, nil
}
