// Command server runs the Kane Poker Quality console: a server-rendered
// Go + htmx app that turns the DevFun Arena poker agent's own policy and
// review data into a checkable quality surface. See docs/PLAN.md.
package main

import (
	"bytes"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/udingethe/kane-poker-quality/internal/policy"
	"github.com/udingethe/kane-poker-quality/internal/quality"
	"github.com/udingethe/kane-poker-quality/internal/review"
	"github.com/udingethe/kane-poker-quality/internal/store"
)

type app struct {
	tmpl         *template.Template
	dataStore    *store.Store
	surface      quality.Surface
	policyReport policy.Report
	reviewQueue  *review.Queue
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

	a := &app{
		tmpl:         tmpl,
		dataStore:    s,
		surface:      quality.Build(s.Quality),
		policyReport: policy.Run(s.Decisions.RecentDecisions, s.Policy),
		reviewQueue:  review.NewQueue(s.Quality.DisagreementsSample),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleHome)
	mux.HandleFunc("/policy", a.handlePolicy)
	mux.HandleFunc("/review", a.handleReview)
	mux.HandleFunc("/review/", a.handleReviewAction)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	log.Printf("kane-poker-quality listening on %s (data=%s)", addr, dataDir)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
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
// swap in place.
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

	var item review.Item
	for _, it := range a.reviewQueue.All() {
		if it.DecisionID == id {
			item = it
			break
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.tmpl.ExecuteTemplate(w, "review-row", item); err != nil {
		log.Printf("render review-row: %v", err)
	}
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
