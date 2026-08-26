// Command server runs the Sekats console: a server-rendered
// Go + htmx app that turns the DevFun Arena poker agent's own policy and
// review data into a checkable quality surface. See docs/PLAN.md.
package main

import (
	"bytes"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/udingethe/sekats/internal/policy"
	"github.com/udingethe/sekats/internal/quality"
	"github.com/udingethe/sekats/internal/review"
	"github.com/udingethe/sekats/internal/store"
	"github.com/udingethe/sekats/internal/trends"
	"github.com/udingethe/sekats/internal/verdicts"
)

type app struct {
	tmpl         *template.Template
	dataStore    *store.Store
	dataDir      string
	surface      quality.Surface
	policyReport policy.Report
	reviewQueue  *review.Queue
	trendsView   trends.View
	vStore       *verdicts.Store
}

func main() {
	dataDir := envOr("DATA_DIR", "data")
	tmplDir := envOr("TEMPLATE_DIR", "templates")
	staticDir := envOr("STATIC_DIR", "static")
	addr := envOr("ADDR", ":8080")

	s, err := store.Load(dataDir)
	if err != nil {
		log.Fatalf("failed to load fixture data: %v", err)
	}

	tmpl, err := template.ParseGlob(filepath.Join(tmplDir, "*.html"))
	if err != nil {
		log.Fatalf("failed to parse templates: %v", err)
	}

	vs := verdicts.NewStore(filepath.Join(dataDir, "verdicts.json"))
	if err := vs.Load(); err != nil {
		log.Printf("loading verdicts: %v", err)
	}

	a := &app{
		tmpl:         tmpl,
		dataStore:    s,
		dataDir:      dataDir,
		surface:      quality.Build(s.Quality),
		policyReport: policy.Run(s.Decisions.RecentDecisions, s.Policy),
		reviewQueue:  review.NewQueue(s.Quality.DisagreementsSample),
		trendsView:   trends.Build(s.Decisions.PolicySuggestions, s.Quality),
		vStore:       vs,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleHome)
	mux.HandleFunc("/policy", a.handlePolicy)
	mux.HandleFunc("/review", a.handleReview)
	mux.HandleFunc("/review/", a.handleReviewAction)
	mux.HandleFunc("/trends", a.handleTrends)
	mux.HandleFunc("/export", a.handleExport)
	mux.HandleFunc("/verdicts", a.handleVerdicts)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	log.Printf("sekats listening on %s (data=%s)", addr, dataDir)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal()
	}
}

func (a *app) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	a.renderPage(w, "page-home", "Quality Surface", struct {
		Surface quality.Surface
	}{Surface: a.surface})
}

func (a *app) handlePolicy(w http.ResponseWriter, r *http.Request) {
	a.renderPage(w, "page-policy", "Policy-as-ACs", struct {
		Report policy.Report
	}{Report: a.policyReport})
}

func (a *app) handleReview(w http.ResponseWriter, r *http.Request) {
	pending, accepted, rejected := a.reviewQueue.Counts()
	a.renderPage(w, "page-review", "Review Queue", struct {
		Items    []review.Item
		Pending  int
		Accepted int
		Rejected int
	}{
		Items:    a.reviewQueue.All(),
		Pending:  pending,
		Accepted: accepted,
		Rejected: rejected,
	})
}

// handleReviewAction handles POST /review/{id}/accept and
// POST /review/{id}/reject, returning the updated row partial for htmx to
// swap in place. Also persists the verdict via the verdicts store.
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

	if !a.reviewQueue.SetVerdict(id, verdict) {
		http.NotFound(w, r)
		return
	}

	// Persist the verdict so it survives restarts and feeds the /verdicts report.
	if err := a.vStore.Record(id, string(verdict)); err != nil {
		log.Printf("persist verdict: %v", err)
	}

	var item review.Item
	for _, it := range a.reviewQueue.All() {
		if it.DecisionID == id {
			item = it
			break
		}
	}
	pending, accepted, rejected := a.reviewQueue.Counts()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err = a.tmpl.ExecuteTemplate(w, "review-action-response", struct {
		Item   review.Item
		Counts struct {
			Pending  int
			Accepted int
			Rejected int
		}
	}{
		Item: item,
		Counts: struct {
			Pending  int
			Accepted int
			Rejected int
		}{Pending: pending, Accepted: accepted, Rejected: rejected},
	})
	if err != nil {
		log.Printf("render review-action-response: %v", err)
	}
}

// handleTrends renders the /trends screen with the suggestion timeline and
// disagreement-action matrix.
func (a *app) handleTrends(w http.ResponseWriter, r *http.Request) {
	a.renderPage(w, "page-trends", "Trends", a.trendsView)
}

// handleExport serves the current review queue state as downloadable JSON.
// GET without ?download=true shows a preview page; ?download=true triggers
// a file download.
func (a *app) handleExport(w http.ResponseWriter, r *http.Request) {
	items := a.reviewQueue.All()
	pending, accepted, rejected := a.reviewQueue.Counts()

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
// page built from the persisted verdicts and the disagreement fixture rows.
func (a *app) handleVerdicts(w http.ResponseWriter, r *http.Request) {
	reportMarkdown, _ := verdicts.BuildReport(a.dataStore.Quality.DisagreementsSample, a.vStore.All())
	// Render markdown as plain text in a pre tag for simplicity; a full
	// markdown-to-HTML renderer would be overkill for a hackathon demo.
	htmlContent := `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<title>Verdict Report — Sekats</title>
<style>
  body{background:#0f1115;color:#e6e8ec;font-family:sans-serif;margin:0;padding:32px;}
  pre{background:#171a21;border:1px solid #2a2e38;border-radius:8px;padding:20px;font-size:13px;white-space:pre-wrap;max-width:800px;line-height:1.6;}
  h1{margin-bottom:4px;}
  .subtitle{color:#8b90a0;margin-top:0;margin-bottom:24px;}
</style></head><body>
<h1>Verdict Report</h1>
<p class="subtitle">Human adjudications joined against the frozen fixture — generated at ` + time.Now().UTC().Format("2006-01-02 15:04 UTC") + `</p>
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
