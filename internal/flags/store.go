package flags

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	StatusNotConfigured = "not_configured"
	StatusError         = "error"
	StatusRunning       = "running"
	StatusPaused        = "paused"
)

// Rollout is sekats' durable local projection of a Flagsmith experiment. It
// records local replay evidence immediately, even while the warehouse-backed
// significance calculation is still pending.
type Rollout struct {
	DraftID           string        `json:"draft_id"`
	FlagName          string        `json:"flag_name"`
	Status            string        `json:"status"`
	Error             string        `json:"error,omitempty"`
	FeatureID         int           `json:"feature_id,omitempty"`
	VariantID         int           `json:"variant_id,omitempty"`
	ExperimentID      int           `json:"experiment_id,omitempty"`
	AgreementMetricID int           `json:"agreement_metric_id,omitempty"`
	ViolationMetricID int           `json:"violation_metric_id,omitempty"`
	DashboardURL      string        `json:"dashboard_url,omitempty"`
	RolloutPercent    float64       `json:"rollout_percent,omitempty"`
	CandidatePercent  float64       `json:"candidate_percent,omitempty"`
	Summary           ReplaySummary `json:"summary"`
	Rows              []ReplayRow   `json:"rows,omitempty"`
	Results           *Results      `json:"results,omitempty"`
	CandidateWinPct   float64       `json:"candidate_win_pct,omitempty"`
	CandidateWinOK    bool          `json:"candidate_win_ok,omitempty"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
}

type ReplayRow struct {
	DecisionID   int64  `json:"decision_id"`
	Identifier   string `json:"identifier"`
	Variant      string `json:"variant"`
	Enrolled     bool   `json:"enrolled"`
	ShadowAction string `json:"shadow_action"`
	Agree        bool   `json:"agree"`
	Violations   int    `json:"violations"`
	ExperimentID int    `json:"experiment_id,omitempty"`
	Source       string `json:"source"`
	Error        string `json:"error,omitempty"`
}

type ReplaySummary struct {
	Replayed     int            `json:"replayed"`
	Enrolled     int            `json:"enrolled"`
	ExposureSent int            `json:"exposure_sent"`
	Agree        map[string]int `json:"agree"`
	Violations   map[string]int `json:"violations"`
	Total        map[string]int `json:"total"`
	Errors       int            `json:"errors"`
	LastReplayAt time.Time      `json:"last_replay_at,omitempty"`
}

func emptySummary() ReplaySummary {
	return ReplaySummary{
		Agree:      map[string]int{Control: 0, Candidate: 0},
		Violations: map[string]int{Control: 0, Candidate: 0},
		Total:      map[string]int{Control: 0, Candidate: 0},
	}
}

// Store persists rollout projections beside the rest of sekats' state. It is
// JSON rather than a database so operator evidence remains inspectable and
// portable in the hackathon demo.
type Store struct {
	path string
	mu   sync.Mutex
	rows map[string]Rollout
}

func NewStore(path string) *Store {
	return &Store{path: path, rows: make(map[string]Rollout)}
}

func (s *Store) Load() error {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var rows map[string]Rollout
	if err := json.Unmarshal(raw, &rows); err != nil {
		return err
	}
	if rows != nil {
		s.rows = rows
	}
	return nil
}

func (s *Store) Get(draftID string) (Rollout, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[draftID]
	return r, ok
}

func (s *Store) All() []Rollout {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Rollout, 0, len(s.rows))
	for _, r := range s.rows {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DraftID < out[j].DraftID })
	return out
}

func (s *Store) Put(r Rollout) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if old, ok := s.rows[r.DraftID]; ok && r.CreatedAt.IsZero() {
		r.CreatedAt = old.CreatedAt
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	if len(r.Summary.Agree) == 0 {
		r.Summary = emptySummary()
	}
	s.rows[r.DraftID] = r
	return s.writeLocked()
}

func (s *Store) writeLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s.rows, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
