// Package live is a read-only bridge to the agent's SQLite decision log.
//
// The database runs on the agent's host and is written in WAL mode by the
// live process. This package opens a locally-synced snapshot — a
// WAL-consistent copy made with SQLite's online-backup API, see
// tools/sync_live.sh — with the connection forced read-only (URI mode=ro
// plus PRAGMA query_only), and derives exactly the stats the frozen JSON
// fixtures carry: suggestion totals, review error buckets, linked-review
// agreement, the disagreement sample, and the recent decisions.
//
// The console therefore runs on fresh data instead of a stale export: the
// broken-loop callout on the Quality Surface reflects the current state of
// the log, not an August export. When no snapshot is present the server
// falls back to the frozen fixtures and says so in the UI.
//
// Schema note: the queries below speak the poker schema
// (our_decisions / llm_reviews / policy_suggestions, as created by the
// poker agent's arena_runner.py). The layers above this package — engine,
// adapters, promotion — are schema-neutral; a new agent with a different
// log schema adds its own loader here (or its own package) behind the same
// *store.Store shape.
package live

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sneldao/sekats/internal/store"
)

// SourceInfo describes where the data came from and how fresh it is.
type SourceInfo struct {
	Kind             string // "live"
	Path             string // the snapshot file that was opened
	SyncedAt         time.Time
	LatestDecisionID int64
	LatestDecisionAt time.Time
}

// Load opens the snapshot at dbPath read-only and derives the store the
// app serves. It fails if the file is missing or not a readable SQLite
// database with the expected tables — callers treat that as "fall back to
// the frozen fixtures".
func Load(dbPath string) (*store.Store, *SourceInfo, error) {
	info := &SourceInfo{Kind: "live", Path: dbPath}
	fi, err := os.Stat(dbPath)
	if err != nil {
		return nil, nil, err
	}
	info.SyncedAt = fi.ModTime()

	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()

	// Defense in depth: the URI already asks for read-only; query_only makes
	// any accidental write statement fail too.
	if _, err := db.Exec("PRAGMA query_only = ON"); err != nil {
		return nil, nil, fmt.Errorf("open read-only: %w", err)
	}

	s := &store.Store{}
	if err := loadDecisions(db, &s.Decisions, info); err != nil {
		return nil, nil, err
	}
	if err := loadQuality(db, &s.Quality); err != nil {
		return nil, nil, err
	}
	return s, info, nil
}

// loadDecisions mirrors poker/tools/export_scenarios.py: the recent
// decisions the AC engine runs against, and the newest policy suggestions
// for the trends timeline.
func loadDecisions(db *sql.DB, out *store.DecisionsExport, info *SourceInfo) error {
	rows, err := db.Query(`
		SELECT id, decided_at, mode, street, hand_code, COALESCE(board_json, ''), pot,
		       COALESCE(legal_json, ''), action, amount, COALESCE(source, ''), accepted, error
		FROM our_decisions
		ORDER BY id DESC
		LIMIT 80`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var d store.Decision
		if err := rows.Scan(&d.ID, &d.DecidedAt, &d.Mode, &d.Street, &d.HandCode,
			&d.BoardJSON, &d.Pot, &d.LegalJSON, &d.Action, &d.Amount,
			&d.Source, &d.Accepted, &d.Error); err != nil {
			return err
		}
		out.RecentDecisions = append(out.RecentDecisions, d)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	rows, err = db.Query(`
		SELECT created_at, COALESCE(model, ''), COALESCE(summary, ''), status, error,
		       COALESCE(suggestion_json, '')
		FROM policy_suggestions
		ORDER BY id DESC
		LIMIT 10`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var p store.PolicySuggestion
		if err := rows.Scan(&p.CreatedAt, &p.Model, &p.Summary, &p.Status,
			&p.Error, &p.SuggestionJSON); err != nil {
			return err
		}
		out.PolicySuggestions = append(out.PolicySuggestions, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	var latestID, latestAt int64
	if err := db.QueryRow(`SELECT MAX(id), MAX(decided_at) FROM our_decisions`).
		Scan(&latestID, &latestAt); err != nil {
		return err
	}
	info.LatestDecisionID = latestID
	if latestAt > 0 {
		info.LatestDecisionAt = time.UnixMilli(latestAt).UTC()
	}
	return nil
}

// loadQuality recomputes every number in quality_stats.json from the raw
// tables, using the same semantics the fixture export used (linked =
// decision_id IS NOT NULL; agreement only over linked, error-free
// reviews; error buckets over all reviews, "clean" for no error).
func loadQuality(db *sql.DB, q *store.QualityStats) error {
	if err := db.QueryRow(`SELECT COUNT(*) FROM policy_suggestions`).
		Scan(&q.PolicySuggestionsTotal); err != nil {
		return err
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM policy_suggestions WHERE status = 'promoted'`).
		Scan(&q.PolicySuggestionsPromoted); err != nil {
		return err
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM llm_reviews`).
		Scan(&q.LLMReviewsTotal); err != nil {
		return err
	}

	rows, err := db.Query(`
		SELECT COALESCE(NULLIF(error, ''), 'clean') AS bucket, COUNT(*)
		FROM llm_reviews
		GROUP BY bucket
		ORDER BY COUNT(*) DESC, bucket`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var b store.ErrorBucket
		if err := rows.Scan(&b.Bucket, &b.Count); err != nil {
			rows.Close()
			return err
		}
		q.LLMReviewsByError = append(q.LLMReviewsByError, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM llm_reviews WHERE decision_id IS NOT NULL`).
		Scan(&q.LinkedReviews); err != nil {
		return err
	}

	rows, err = db.Query(`
		SELECT od.action, lr.action, COUNT(*)
		FROM llm_reviews lr
		JOIN our_decisions od ON od.id = lr.decision_id
		WHERE lr.decision_id IS NOT NULL AND COALESCE(lr.error, '') = ''
		GROUP BY od.action, lr.action
		ORDER BY COUNT(*) DESC, od.action, lr.action`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p store.AgreementPair
		if err := rows.Scan(&p.OurAction, &p.LLMAction, &p.Count); err != nil {
			rows.Close()
			return err
		}
		q.AgreementBreakdown = append(q.AgreementBreakdown, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	if err := db.QueryRow(`
		SELECT SUM(CASE WHEN od.action = lr.action THEN 1 ELSE 0 END), COUNT(*)
		FROM llm_reviews lr
		JOIN our_decisions od ON od.id = lr.decision_id
		WHERE lr.decision_id IS NOT NULL AND COALESCE(lr.error, '') = ''`).
		Scan(&q.AgreementRate.Agree, &q.AgreementRate.Total); err != nil {
		return err
	}

	rows, err = db.Query(`
		SELECT hand_code, action, COUNT(*)
		FROM our_decisions
		WHERE street = 'Preflop'
		GROUP BY hand_code, action
		ORDER BY COUNT(*) DESC, hand_code, action
		LIMIT 20`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p store.PreflopActionCount
		if err := rows.Scan(&p.HandCode, &p.Action, &p.Count); err != nil {
			rows.Close()
			return err
		}
		q.PreflopActionDist = append(q.PreflopActionDist, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	rows, err = db.Query(`
		SELECT od.action, lr.action, od.hand_code, od.street, od.pot,
		       COALESCE(lr.response_text, ''), od.id
		FROM llm_reviews lr
		JOIN our_decisions od ON od.id = lr.decision_id
		WHERE lr.decision_id IS NOT NULL
		  AND COALESCE(lr.error, '') = ''
		  AND od.action != lr.action
		ORDER BY lr.id DESC
		LIMIT 25`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var d store.Disagreement
		if err := rows.Scan(&d.OurAction, &d.LLMAction, &d.HandCode, &d.Street,
			&d.Pot, &d.ResponseText, &d.DecisionID); err != nil {
			rows.Close()
			return err
		}
		q.DisagreementsSample = append(q.DisagreementsSample, d)
	}
	rows.Close()
	return rows.Err()
}
