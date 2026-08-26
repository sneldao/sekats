package live

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// seedSchema builds a temporary database with the poker agent's schema
// (as created by arena_runner.py) and a small set of known rows.
func seedSchema(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "arena-poker.sqlite3")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	schema := `
	CREATE TABLE our_decisions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		decided_at INTEGER NOT NULL,
		mode TEXT, competition_id TEXT, table_id TEXT,
		street TEXT, hand_code TEXT, board_json TEXT,
		pot INTEGER, legal_json TEXT,
		action TEXT, amount INTEGER, source TEXT,
		reasoning TEXT, accepted INTEGER, error TEXT
	);
	CREATE TABLE llm_reviews (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at INTEGER NOT NULL,
		mode TEXT, table_id TEXT, model TEXT, prompt_json TEXT,
		response_text TEXT, action TEXT, amount INTEGER,
		confidence REAL, latency_ms INTEGER, used INTEGER DEFAULT 0,
		error TEXT, decision_id INTEGER
	);
	CREATE TABLE policy_suggestions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at INTEGER NOT NULL,
		mode TEXT, table_ids_json TEXT, model TEXT, prompt_json TEXT,
		suggestion_json TEXT, summary TEXT,
		status TEXT DEFAULT 'pending', error TEXT
	);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	decisions := []struct {
		id, at    int64
		street    string
		hand      string
		action    string
		source    string
		pot       int
		boardJSON string
	}{
		{1, 1000, "Preflop", "A7o", "raise", "deterministic", 5, ""},
		{2, 2000, "Flop", "K5s", "bet", "mixed", 12, ""},
		{3, 3000, "Turn", "A7o", "bet", "mixed", 9, ""},
	}
	for _, d := range decisions {
		if _, err := db.Exec(
			`INSERT INTO our_decisions (id,decided_at,mode,street,hand_code,board_json,pot,legal_json,action,source,accepted)
			 VALUES (?,?,'eval',?,?,?,?,?,?,?,1)`,
			d.id, d.at, d.street, d.hand, d.boardJSON, d.pot, "[]", d.action, d.source); err != nil {
			t.Fatal(err)
		}
	}

	reviews := []struct {
		id, at     int64
		decisionID *int64
		action     string
		response   string
		err        string
	}{
		{1, 2100, int64p(2), "check", `{"action": "check", "rationale": "weak hand"}`, ""},
		{2, 3100, int64p(3), "bet", `{"action": "bet",   "rationale": "value"}`, ""},
		{3, 1100, int64p(1), "fold", ``, "HTTP Error 403: Forbidden"},
		{4, 4000, nil, "check", ``, ""}, // unlinked historical row: NULL, not 0
	}
	for _, r := range reviews {
		if _, err := db.Exec(
			`INSERT INTO llm_reviews (id,created_at,decision_id,action,response_text,error)
			 VALUES (?,?,?,?,?,?)`,
			r.id, r.at, r.decisionID, r.action, r.response, r.err); err != nil {
			t.Fatal(err)
		}
	}

	suggestions := []struct {
		id     int64
		at     int64
		status string
		err    string
	}{
		{1, 1000, "pending", ""},
		{2, 2000, "promoted", ""},
		{3, 3000, "error", "boom"},
	}
	for _, s := range suggestions {
		if _, err := db.Exec(
			`INSERT INTO policy_suggestions (id,created_at,model,suggestion_json,summary,status,error)
			 VALUES (?,?,'m1','{}',?,?,?)`,
			s.id, s.at, "sum-"+s.status, s.status, s.err); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestLoadDerivesFixtureSemantics(t *testing.T) {
	st, info, err := Load(seedSchema(t))
	if err != nil {
		t.Fatal(err)
	}

	// Source info
	if info.Kind != "live" {
		t.Fatalf("kind = %q", info.Kind)
	}
	if info.LatestDecisionID != 3 {
		t.Fatalf("latest decision id = %d, want 3", info.LatestDecisionID)
	}
	if !info.LatestDecisionAt.Equal(time.UnixMilli(3000).UTC()) {
		t.Fatalf("latest decision at = %v", info.LatestDecisionAt)
	}
	if info.SyncedAt.IsZero() {
		t.Fatal("synced at is zero")
	}

	// Recent decisions: latest 80, newest first, with ids and timestamps.
	got := st.Decisions
	if len(got.RecentDecisions) != 3 {
		t.Fatalf("recent decisions = %d, want 3", len(got.RecentDecisions))
	}
	if got.RecentDecisions[0].ID != 3 || got.RecentDecisions[0].HandCode != "A7o" || got.RecentDecisions[0].DecidedAt != 3000 {
		t.Fatalf("newest decision = %+v", got.RecentDecisions[0])
	}
	if len(got.PolicySuggestions) != 3 {
		t.Fatalf("suggestions = %d, want 3", len(got.PolicySuggestions))
	}

	// Suggestion totals — the broken-loop numbers.
	if st.Quality.PolicySuggestionsTotal != 3 || st.Quality.PolicySuggestionsPromoted != 1 {
		t.Fatalf("suggestion totals = %d/%d, want 3/1",
			st.Quality.PolicySuggestionsTotal, st.Quality.PolicySuggestionsPromoted)
	}

	// Error buckets over ALL reviews, "clean" for no error.
	buckets := make(map[string]int)
	for _, b := range st.Quality.LLMReviewsByError {
		buckets[b.Bucket] = b.Count
	}
	if buckets["clean"] != 3 || buckets["HTTP Error 403: Forbidden"] != 1 || len(st.Quality.LLMReviewsByError) != 2 {
		t.Fatalf("error buckets = %v", st.Quality.LLMReviewsByError)
	}
	if st.Quality.LLMReviewsTotal != 4 {
		t.Fatalf("llm reviews total = %d, want 4", st.Quality.LLMReviewsTotal)
	}

	// Linked = decision_id IS NOT NULL (regardless of error).
	if st.Quality.LinkedReviews != 3 {
		t.Fatalf("linked reviews = %d, want 3", st.Quality.LinkedReviews)
	}

	// Agreement over linked, error-free reviews only.
	if st.Quality.AgreementRate.Agree != 1 || st.Quality.AgreementRate.Total != 2 {
		t.Fatalf("agreement = %d/%d, want 1/2",
			st.Quality.AgreementRate.Agree, st.Quality.AgreementRate.Total)
	}

	// Disagreement sample: linked, error-free, action mismatch, newest first.
	ds := st.Quality.DisagreementsSample
	if len(ds) != 1 {
		t.Fatalf("disagreements = %d, want 1", len(ds))
	}
	d := ds[0]
	if d.DecisionID != 2 || d.OurAction != "bet" || d.LLMAction != "check" ||
		d.HandCode != "K5s" || d.Street != "Flop" || d.Pot != 12 {
		t.Fatalf("disagreement = %+v", d)
	}
	if d.ResponseText != `{"action": "check", "rationale": "weak hand"}` {
		t.Fatalf("response text = %q", d.ResponseText)
	}
}

func TestLoadReadOnly(t *testing.T) {
	path := seedSchema(t)
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO our_decisions (decided_at,street,hand_code,action) VALUES (1,'Flop','99','bet')`); err == nil {
		t.Fatal("write through the read-only bridge succeeded")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, _, err := Load(filepath.Join(t.TempDir(), "nope.sqlite3")); err == nil {
		t.Fatal("expected error for missing snapshot")
	}
}

func TestLoadNotADatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "junk.sqlite3")
	if err := os.WriteFile(path, []byte("this is not a sqlite database"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(path); err == nil {
		t.Fatal("expected error for non-sqlite file")
	}
}

func int64p(v int64) *int64 { return &v }
