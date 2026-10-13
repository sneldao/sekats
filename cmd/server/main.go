// Command server runs the Sekats console: a server-rendered Go + htmx app
// that turns an autonomous agent's own policy and review data into a
// checkable quality surface. See docs/PLAN.md.
//
// Data source: by default the console serves the frozen fixture export in
// data/. When a synced snapshot of the agent's live SQLite log is present
// (data/live/arena-poker.sqlite3 by default, or the path in SEKATS_DB —
// produced by tools/sync_live.sh), it opens the snapshot read-only and
// refreshes whenever the file changes, so the screens reflect the current
// state of the log rather than a stale export. SEKATS_DB=off forces
// fixture mode.
//
// Adapter: SEKATS_ADAPTER selects the domain adapter (default "poker");
// it supplies the policy list lookups, the declarative rules, the pattern
// buckets, and the drafted policy edits.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sneldao/sekats/internal/adapters"
	"github.com/sneldao/sekats/internal/adapters/poker"
	"github.com/sneldao/sekats/internal/attest"
	"github.com/sneldao/sekats/internal/engine"
	"github.com/sneldao/sekats/internal/live"
	"github.com/sneldao/sekats/internal/monad"
	"github.com/sneldao/sekats/internal/policy"
	"github.com/sneldao/sekats/internal/promotion"
	"github.com/sneldao/sekats/internal/quality"
	"github.com/sneldao/sekats/internal/review"
	"github.com/sneldao/sekats/internal/store"
	"github.com/sneldao/sekats/internal/trends"
	"github.com/sneldao/sekats/internal/verdicts"
)

type app struct {
	tmpl             *template.Template
	dataDir          string
	adapter          adapters.Adapter
	data             *dataSource
	vStore           *verdicts.Store
	promos           *promotion.Store
	promoteThreshold int
	anchorer         attest.Anchorer // nil = on-chain anchoring disabled
	attests          *attest.Store
	attestExplorer   string // base URL for tx links, even when signing is off
	attestContract   string // read-only config, independent of signing
	publicRPC        string
	treasury         *monad.TreasuryReader
}

// dataSource is the current view over the agent's data. In live mode the
// underlying snapshot file is re-checked on every request and reloaded
// (stats, trends, policy report, and a review queue re-seeded with the
// persisted verdicts) when its mtime changes; in fixture mode it is static.
type dataSource struct {
	mu       sync.Mutex
	dataDir  string
	livePath string // "" = fixture mode
	lastSync time.Time
	verdicts *verdicts.Store

	st    *store.Store
	surf  quality.Surface
	src   quality.SourceInfo
	rep   policy.Report
	trend trends.View
	queue *review.Queue
}

func main() {
	dataDir := envOr("DATA_DIR", "data")
	tmplDir := envOr("TEMPLATE_DIR", "templates")
	staticDir := envOr("STATIC_DIR", "static")
	addr := envOr("ADDR", ":8080")

	ad, err := adapters.Get(envOr("SEKATS_ADAPTER", poker.Name))
	if err != nil {
		log.Fatalf("adapter: %v", err)
	}

	ds, err := newDataSource(dataDir, ad)
	if err != nil {
		log.Fatalf("failed to load data: %v", err)
	}

	tmpl, err := template.ParseGlob(filepath.Join(tmplDir, "*.html"))
	if err != nil {
		log.Fatalf("failed to parse templates: %v", err)
	}

	threshold := 3
	if v := os.Getenv("SEKATS_PROMOTE_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			threshold = n
		}
	}

	a := &app{
		tmpl:    tmpl,
		dataDir: dataDir,
		adapter: ad,
		data:    ds,
		vStore:  ds.verdicts,
		promos: promotion.NewStore(
			filepath.Join(dataDir, "pattern_rejections.json"),
			filepath.Join(dataDir, "policy_drafts.json"),
			ad.NewDraft,
		),
		promoteThreshold: threshold,
		attests:          attest.NewStore(filepath.Join(dataDir, "attestations.json")),
		attestExplorer:   envOr("SEKATS_EXPLORER", "https://monadscan.com"),
		attestContract:   os.Getenv("SEKATS_ATTEST_CONTRACT"),
		publicRPC:        envOr("SEKATS_PUBLIC_RPC", "https://rpc.monad.xyz"),
		treasury:         monad.NewTreasuryReader(envOr("SEKATS_RPC", "https://rpc.monad.xyz"), os.Getenv("SEKATS_AGENT_ADDRESS")),
	}
	if err := a.promos.Load(); err != nil {
		log.Printf("loading promotion state: %v", err)
	}
	if err := a.attests.Load(); err != nil {
		log.Printf("loading attestations: %v", err)
	}
	if anch, err := attest.FromEnv(); err != nil {
		log.Printf("attestation: %v", err)
	} else if anch != nil {
		a.anchorer = anch
		log.Printf("attestation: anchoring on %s via %s", anch.ChainName(), anch.Contract())
	} else {
		log.Printf("attestation: no signer configured — on-chain anchoring disabled")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleHome)
	mux.HandleFunc("/policy", a.handlePolicy)
	mux.HandleFunc("/review", a.handleReview)
	mux.HandleFunc("/review/", a.handleReviewAction)
	mux.HandleFunc("/trends", a.handleTrends)
	mux.HandleFunc("/export", a.handleExport)
	mux.HandleFunc("/verdicts", a.handleVerdicts)
	mux.HandleFunc("/promotions", a.handlePromotions)
	mux.HandleFunc("/promotions/", a.handlePromotionAction)
	mux.HandleFunc("/anchors", a.handleAnchors)
	mux.HandleFunc("/api/anchors", a.handleAnchorsJSON)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	log.Printf("sekats listening on %s (data=%s, adapter=%s, source=%s)", addr, dataDir, ad.Name(), ds.src.Kind)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal()
	}
}

func init() {
	adapters.Register(poker.New())
}

// newDataSource loads fixtures or the live bridge and derives the initial
// screens. Live mode: SEKATS_DB (or the default data/live/arena-poker.sqlite3)
// when the file exists; SEKATS_DB=off forces fixtures.
func newDataSource(dataDir string, ad adapters.Adapter) (*dataSource, error) {
	vs := verdicts.NewStore(filepath.Join(dataDir, "verdicts.json"))
	if err := vs.Load(); err != nil {
		log.Printf("loading verdicts: %v", err)
	}
	ds := &dataSource{dataDir: dataDir, verdicts: vs}

	dbPath := ""
	switch v := os.Getenv("SEKATS_DB"); {
	case v == "off":
		// operator forced fixture mode; no auto-detect
	case v != "":
		dbPath = v
	default:
		// unset: auto-detect a synced snapshot in data/live/
		if p := filepath.Join(dataDir, "live", "arena-poker.sqlite3"); statOK(p) {
			dbPath = p
		}
	}

	if dbPath != "" {
		if st, info, err := live.Load(dbPath); err != nil {
			log.Printf("live bridge: %v — falling back to frozen fixtures", err)
		} else {
			fi, _ := os.Stat(dbPath)
			ds.livePath = dbPath
			if fi != nil {
				ds.lastSync = fi.ModTime()
			}
			ds.st = st
			ds.src = sourceInfoLive(info)
			log.Printf("live bridge: read-only %s (latest decision #%d)", dbPath, info.LatestDecisionID)
			ds.rebuild(ad)
			return ds, nil
		}
	}

	st, err := store.Load(dataDir)
	if err != nil {
		return nil, err
	}
	ds.st = st
	fi, _ := os.Stat(filepath.Join(dataDir, "quality_stats.json"))
	freshness := "frozen export"
	if fi != nil {
		freshness = "frozen export, file date " + fi.ModTime().UTC().Format("2006-01-02")
	}
	latestID := int64(0)
	for _, d := range st.Quality.DisagreementsSample {
		if d.DecisionID > latestID {
			latestID = d.DecisionID
		}
	}
	ds.src = quality.SourceInfo{Kind: "fixture", Freshness: freshness, LatestDecisionID: latestID}
	ds.rebuild(ad)
	return ds, nil
}

func statOK(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sourceInfoLive(info *live.SourceInfo) quality.SourceInfo {
	return quality.SourceInfo{
		Kind:             "live",
		Freshness:        "synced " + info.SyncedAt.UTC().Format("2006-01-02 15:04 MST"),
		LatestDecisionID: info.LatestDecisionID,
		LatestDecisionAt: info.LatestDecisionAt.Format("2006-01-02 15:04 UTC"),
	}
}

// rebuild re-derives every screen from the current store, re-seeding the
// review queue with the persisted verdicts. Caller holds ds.mu.
func (ds *dataSource) rebuild(ad adapters.Adapter) {
	ds.surf = quality.Build(ds.st.Quality)
	if rep, err := policy.Run(ad, ds.dataDir, ds.st.Decisions.RecentDecisions); err != nil {
		log.Printf("policy rules: %v", err)
		ds.rep = policy.Report{TotalDecisions: len(ds.st.Decisions.RecentDecisions)}
	} else {
		ds.rep = rep
	}
	ds.trend = trends.Build(ds.st.Decisions.PolicySuggestions, ds.st.Quality)
	ds.queue = review.NewQueue(ds.st.Quality.DisagreementsSample)
	for _, e := range ds.verdicts.All() {
		ds.queue.SetVerdict(e.DecisionID, review.Verdict(e.Verdict))
	}
}

// Refresh re-checks the live snapshot (mtime) and reloads if it changed.
// Safe to call on every request; a no-op in fixture mode. Failures keep
// the previous data — a half-written sync file must never blank the UI.
func (ds *dataSource) Refresh(ad adapters.Adapter) {
	if ds.livePath == "" {
		return
	}
	fi, err := os.Stat(ds.livePath)
	if err != nil {
		return
	}
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if ds.lastSync.Equal(fi.ModTime()) {
		return
	}
	st, info, err := live.Load(ds.livePath)
	if err != nil {
		log.Printf("live bridge refresh: %v — keeping previous data", err)
		return
	}
	ds.lastSync = fi.ModTime()
	ds.st = st
	ds.src = sourceInfoLive(info)
	log.Printf("live bridge: refreshed (latest decision #%d)", info.LatestDecisionID)
	ds.rebuild(ad)
}

// Snapshot returns a consistent read of the current screens.
func (ds *dataSource) Snapshot() (*store.Store, quality.Surface, quality.SourceInfo, policy.Report, trends.View, *review.Queue) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.st, ds.surf, ds.src, ds.rep, ds.trend, ds.queue
}

func (a *app) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	a.data.Refresh(a.adapter)
	_, surf, src, _, _, _ := a.data.Snapshot()
	staged, approved, _ := a.promos.Counts()
	a.renderPage(w, "page-home", "Quality Surface", struct {
		Surface    quality.Surface
		Source     quality.SourceInfo
		Treasury   monad.Treasury
		Promotions struct {
			Staged   int
			Approved int
			Anchored int
			AnchorOn bool
		}
	}{
		Surface:  surf,
		Source:   src,
		Treasury: a.treasury.Snapshot(r.Context()),
		Promotions: struct {
			Staged   int
			Approved int
			Anchored int
			AnchorOn bool
		}{Staged: staged, Approved: approved, Anchored: a.attests.AnchoredCount(), AnchorOn: a.anchorer != nil},
	})
}

func (a *app) handlePolicy(w http.ResponseWriter, r *http.Request) {
	a.data.Refresh(a.adapter)
	_, _, _, rep, _, _ := a.data.Snapshot()
	a.renderPage(w, "page-policy", "Policy-as-ACs", struct {
		Report policy.Report
	}{Report: rep})
}

func (a *app) handleReview(w http.ResponseWriter, r *http.Request) {
	a.data.Refresh(a.adapter)
	_, _, _, _, _, queue := a.data.Snapshot()
	pending, accepted, rejected := queue.Counts()
	a.renderPage(w, "page-review", "Review Queue", struct {
		Items    []review.Item
		Pending  int
		Accepted int
		Rejected int
	}{
		Items:    queue.All(),
		Pending:  pending,
		Accepted: accepted,
		Rejected: rejected,
	})
}

// handleReviewAction handles POST /review/{id}/accept and
// POST /review/{id}/reject, returning the updated row partial for htmx to
// swap in place. Also persists the verdict via the verdicts store, and on
// a rejection feeds the pattern bucket — which auto-drafts a policy edit
// when a repeated pattern crosses the threshold.
func (a *app) handleReviewAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "review" {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		http.Error(w, "invalid decision id", http.StatusBadRequest)
		return
	}

	var verdict review.Verdict
	switch parts[2] {
	case "accept":
		verdict = review.VerdictAccepted
	case "reject":
		verdict = review.VerdictRejected
	default:
		http.NotFound(w, r)
		return
	}

	a.data.Refresh(a.adapter)
	_, _, _, _, _, queue := a.data.Snapshot()

	if !queue.SetVerdict(id, verdict) {
		http.NotFound(w, r)
		return
	}

	// Persist the verdict so it survives restarts and feeds the /verdicts report.
	if err := a.vStore.Record(id, string(verdict)); err != nil {
		log.Printf("persist verdict: %v", err)
	}

	// Rejection side of the loop: bucket the pattern and maybe draft.
	var drafted *engine.Draft
	if verdict == review.VerdictRejected {
		if drafted = a.recordPatternRejection(id); drafted != nil {
			log.Printf("promotion: draft %s staged for pattern %q", drafted.ID, drafted.PatternKey)
		}
	}

	var item review.Item
	for _, it := range queue.All() {
		if it.DecisionID == id {
			item = it
			break
		}
	}
	pending, accepted, rejected := queue.Counts()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err = a.tmpl.ExecuteTemplate(w, "review-action-response", struct {
		Item   review.Item
		Counts struct {
			Pending  int
			Accepted int
			Rejected int
		}
		Draft *engine.Draft
	}{
		Item: item,
		Counts: struct {
			Pending  int
			Accepted int
			Rejected int
		}{Pending: pending, Accepted: accepted, Rejected: rejected},
		Draft: drafted,
	})
	if err != nil {
		log.Printf("render review-action-response: %v", err)
	}
}

// recordPatternRejection buckets one rejected disagreement under the
// adapter's pattern key and records it durably. Returns the auto-drafted
// policy edit, if this rejection crossed the threshold.
func (a *app) recordPatternRejection(id int64) *engine.Draft {
	_, _, _, _, _, queue := a.data.Snapshot()
	var row review.Item
	for _, it := range queue.All() {
		if it.DecisionID == id {
			row = it
			break
		}
	}
	if row.DecisionID == 0 {
		return nil
	}
	key := a.adapter.PatternKey(engine.Review{
		DecisionID:   id,
		Phase:        row.Street,
		ContextCode:  row.HandCode,
		OurAction:    row.OurAction,
		LLMAction:    row.LLMAction,
		Pot:          row.Pot,
		ResponseText: row.ResponseText,
	})
	ev := engine.Evidence{
		DecisionID:  id,
		Phase:       row.Street,
		ContextCode: row.HandCode,
		OurAction:   row.OurAction,
		LLMAction:   row.LLMAction,
		Pot:         row.Pot,
		Rationale:   engine.ExtractRationale(row.ResponseText),
		RecordedAt:  time.Now().UTC(),
	}
	return a.promos.RecordRejection(key, ev, a.promoteThreshold)
}

// handleTrends renders the /trends screen with the suggestion timeline and
// disagreement-action matrix.
func (a *app) handleTrends(w http.ResponseWriter, r *http.Request) {
	a.data.Refresh(a.adapter)
	_, _, _, _, trend, _ := a.data.Snapshot()
	a.renderPage(w, "page-trends", "Trends", trend)
}

// draftCard is the render shape for one policy draft on the promotions
// screen: the draft itself, its pre-rendered patch JSON (shown once
// approved), the adapter's target section, and the on-chain attestation
// state for the draft.
type draftCard struct {
	Draft        engine.Draft
	Patch        string
	Target       string
	Att          attest.Record
	AttOK        bool   // an attestation record exists for this draft
	AnchorOn     bool   // a signer is configured (anchors actually land)
	AttTxURL     string // explorer link for the anchor tx, "" if none
	AttTxShort   string
	AttHashShort string
}

func (a *app) card(d engine.Draft) draftCard {
	patch, _ := json.MarshalIndent(d.Patch(), "", "  ")
	c := draftCard{
		Draft:    d,
		Patch:    string(patch),
		Target:   a.adapter.PolicyTarget(),
		AnchorOn: a.anchorer != nil,
	}
	if rec, ok := a.attests.For(d.ID); ok {
		c.Att, c.AttOK = rec, true
		if len(rec.DocSHA256) > 16 {
			c.AttHashShort = rec.DocSHA256[:16]
		}
		if rec.TxHash != "" {
			c.AttTxShort = rec.TxHash
			if len(rec.TxHash) > 18 {
				c.AttTxShort = rec.TxHash[:18] + "…"
			}
			c.AttTxURL = strings.TrimRight(a.attestExplorer, "/") + "/tx/" + rec.TxHash
		}
	}
	return c
}

// anchorDraft writes the approved draft's attestation doc hash on-chain
// and records the outcome. No-op when no signer is configured — the
// draft is still approved and the UI says anchoring is off.
func (a *app) anchorDraft(d engine.Draft) {
	if a.anchorer == nil || d.Status != engine.DraftApproved {
		return
	}
	if rec, ok := a.attests.For(d.ID); ok && rec.Status == "anchored" {
		return // first approval already anchored; never re-anchor
	}
	doc, err := attest.DocFor(d)
	if err != nil {
		log.Printf("attestation doc: %v", err)
		return
	}
	docBytes, err := doc.CanonicalJSON()
	if err != nil {
		log.Printf("attestation doc marshal: %v", err)
		return
	}
	rec := attest.Record{
		DraftID:   d.ID,
		DocSHA256: fmt.Sprintf("%x", attest.DocHash(docBytes)),
		Chain:     a.anchorer.ChainName(),
		Contract:  a.anchorer.Contract(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	txHash, attester, err := a.anchorer.Anchor(ctx, attest.DraftKey(d.ID), attest.DocHash(docBytes))
	cancel()
	if err != nil {
		rec.Status = "failed"
		rec.Error = err.Error()
		log.Printf("attestation anchor %s: %v", d.ID, err)
	} else {
		now := time.Now().UTC()
		rec.Status = "anchored"
		rec.TxHash = txHash
		rec.Attester = attester
		rec.AnchoredAt = &now
		log.Printf("attestation anchored %s: tx %s on %s", d.ID, txHash, rec.Chain)
	}
	if err := a.attests.Put(rec); err != nil {
		log.Printf("persist attestation: %v", err)
	}
}

// handlePromotions renders the /promotions screen: the staged policy
// edits, the patches approved for application, and the patterns still
// accumulating rejections.
func (a *app) handlePromotions(w http.ResponseWriter, r *http.Request) {
	all := a.promos.AllDrafts()
	cards := make([]draftCard, 0, len(all))
	for _, d := range all {
		cards = append(cards, a.card(d))
	}
	pending, approved, discarded := a.promos.Counts()
	a.renderPage(w, "page-promotions", "Promotion Pipeline", struct {
		Cards     []draftCard
		Pending   int
		Approved  int
		Discarded int
		Threshold int
		Anchored  int
		AnchorOn  bool
		Patterns  []promotion.PatternState
		Target    string
		Agent     string
	}{
		Cards:     cards,
		Pending:   pending,
		Approved:  approved,
		Discarded: discarded,
		Threshold: a.promoteThreshold,
		Anchored:  a.attests.AnchoredCount(),
		AnchorOn:  a.anchorer != nil,
		Patterns:  a.promos.Patterns(),
		Target:    a.adapter.PolicyTarget(),
		Agent:     a.adapter.Agent(),
	})
}

// handlePromotionAction handles POST /promotions/{id}/approve,
// POST /promotions/{id}/discard, POST /promotions/{id}/attest (retry an
// on-chain anchor) — all returning the updated draft card for an htmx
// swap — plus GET /promotions/{id}/patch (the applyable JSON artifact)
// and GET /promotions/{id}/attestation (the byte-stable doc whose
// SHA-256 is anchored on-chain).
func (a *app) handlePromotionAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "promotions" {
		http.NotFound(w, r)
		return
	}
	id := parts[1]

	if r.Method == http.MethodGet {
		d, ok := a.promos.Draft(id)
		if !ok || d.Status != engine.DraftApproved {
			http.NotFound(w, r)
			return
		}
		switch parts[2] {
		case "patch":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Disposition", `attachment; filename="`+id+`.policy-patch.json"`)
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			enc.Encode(d.Patch())
		case "attestation":
			doc, err := attest.DocFor(d)
			if err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			docBytes, err := doc.CanonicalJSON()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Disposition", `attachment; filename="`+id+`.attestation.json"`)
			w.Header().Set("X-Sekats-Doc-SHA256", fmt.Sprintf("%x", attest.DocHash(docBytes)))
			w.Write(docBytes)
		default:
			http.NotFound(w, r)
		}
		return
	}

	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var (
		d   engine.Draft
		err error
	)
	switch parts[2] {
	case "approve":
		d, err = a.promos.Approve(id)
		if err == nil {
			a.anchorDraft(d)
		}
	case "attest":
		var ok bool
		d, ok = a.promos.Draft(id)
		if !ok || d.Status != engine.DraftApproved {
			err = fmt.Errorf("draft %s is not approved", id)
		} else {
			a.anchorDraft(d)
		}
	case "discard":
		d, err = a.promos.Discard(id)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	log.Printf("promotion: draft %s %s", d.ID, d.Status)

	// Re-render the card in place, with the counts updated out of band.
	pending, approved, discarded := a.promos.Counts()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.tmpl.ExecuteTemplate(w, "promotion-card-response", struct {
		Card   draftCard
		Counts struct {
			Pending   int
			Approved  int
			Discarded int
			Threshold int
			Anchored  int
			AnchorOn  bool
		}
	}{
		Card: a.card(d),
		Counts: struct {
			Pending   int
			Approved  int
			Discarded int
			Threshold int
			Anchored  int
			AnchorOn  bool
		}{Pending: pending, Approved: approved, Discarded: discarded, Threshold: a.promoteThreshold, Anchored: a.attests.AnchoredCount(), AnchorOn: a.anchorer != nil},
	}); err != nil {
		log.Printf("render promotion-card-response: %v", err)
	}
}

// handleExport serves the current review queue state as downloadable JSON.
// GET without ?download=true shows a preview page; ?download=true triggers
// a file download.
func (a *app) handleExport(w http.ResponseWriter, r *http.Request) {
	a.data.Refresh(a.adapter)
	_, _, _, _, _, queue := a.data.Snapshot()
	items := queue.All()
	pending, accepted, rejected := queue.Counts()

	exportData := struct {
		ExportedAt string           `json:"exported_at"`
		Pending    int              `json:"pending"`
		Accepted   int              `json:"accepted"`
		Rejected   int              `json:"rejected"`
		Items      []exportItemJSON `json:"items"`
	}{
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Pending:    pending,
		Accepted:   accepted,
		Rejected:   rejected,
		Items:      toExportJSON(items),
	}

	if r.URL.Query().Get("download") == "true" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="sekats-verdict-report.json"`)
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(exportData)
		return
	}

	// Serve a page with a raw JSON preview and a download link.
	htmlContent := `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<title>Verdict Export — Sekats</title>
<style>
  body{background:#0f1115;color:#e6e8ec;font-family:sans-serif;margin:0;padding:32px;}
  a{color:#6ee7b7;}
  pre{background:#171a21;border:1px solid #2a2e38;border-radius:8px;padding:16px;font-size:12px;overflow:auto;max-height:60vh;}
</style></head><body>
<h1>Verdict Export</h1>
<p>Download: <a href="?download=true">sekats-verdict-report.json</a></p>
<pre>` + escapeHTML(mustMarshal(exportData)) + `</pre>
</body></html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(htmlContent))
}

// handleVerdicts renders the human adjudication report as a formatted HTML
// page built from the persisted verdicts and the disagreement rows.
func (a *app) handleVerdicts(w http.ResponseWriter, r *http.Request) {
	a.data.Refresh(a.adapter)
	st, _, src, _, _, _ := a.data.Snapshot()
	reportMarkdown, _ := verdicts.BuildReport(st.Quality.DisagreementsSample, a.vStore.All())
	// Render markdown as plain text in a pre tag for simplicity; a full
	// markdown-to-HTML renderer would be overkill for a console screen.
	htmlContent := `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<title>Verdict Report — Sekats</title>
<style>
  body{background:#0f1115;color:#e6e8ec;font-family:sans-serif;margin:0;padding:32px;}
  pre{background:#171a21;border:1px solid #2a2e38;border-radius:20px;font-size:13px;white-space:pre-wrap;max-width:800px;line-height:1.6;}
  h1{margin-bottom:4px;}
  .subtitle{color:#8b90a0;margin-top:0;margin-bottom:24px;}
</style></head><body>
<h1>Verdict Report</h1>
<p class="subtitle">Human adjudications joined against the ` + src.Kind + ` decision log — generated at ` + time.Now().UTC().Format("2006-01-02 15:04 UTC") + `</p>
<pre>` + escapeHTML(reportMarkdown) + `</pre>
<p style="margin-top:24px"><a href="/export?download=true" style="color:#6ee7b7">Download JSON report</a></p>
</body></html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(htmlContent))
}

// exportItemJSON is the serialisable shape for one review item.
type exportItemJSON struct {
	DecisionID int64   `json:"decision_id"`
	HandCode   string  `json:"hand_code"`
	Street     string  `json:"street"`
	Pot        float64 `json:"pot"`
	OurAction  string  `json:"our_action"`
	LLMAction  string  `json:"llm_action"`
	Response   string  `json:"llm_response"`
	Verdict    string  `json:"verdict"`
}

func toExportJSON(items []review.Item) []exportItemJSON {
	out := make([]exportItemJSON, len(items))
	for i, it := range items {
		out[i] = exportItemJSON{
			DecisionID: it.DecisionID,
			HandCode:   it.HandCode,
			Street:     it.Street,
			Pot:        it.Pot,
			OurAction:  it.OurAction,
			LLMAction:  it.LLMAction,
			Response:   it.ResponseText,
			Verdict:    string(it.Verdict),
		}
	}
	return out
}

func escapeHTML(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;").Replace(s)
}

func mustMarshal(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

// renderPage executes the named page-content template into a buffer, then
// wraps that HTML in the shared layout. Rendering in two passes (rather
// than one template with a dynamic {{template}} name) avoids every page's
// "body" block colliding in the shared template.ParseGlob namespace.
func (a *app) renderPage(w http.ResponseWriter, pageTemplate, title string, pageData any) {
	var buf bytes.Buffer
	if err := a.tmpl.ExecuteTemplate(&buf, pageTemplate, pageData); err != nil {
		log.Printf("render %s: %v", pageTemplate, err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := a.tmpl.ExecuteTemplate(w, "layout", struct {
		Title string
		Body  template.HTML
	}{Title: title, Body: template.HTML(buf.String())})
	if err != nil {
		log.Printf("render layout: %v", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
