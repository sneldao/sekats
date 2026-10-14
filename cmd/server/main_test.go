package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sneldao/sekats/internal/adapters"
	"github.com/sneldao/sekats/internal/adapters/poker"
	"github.com/sneldao/sekats/internal/adapters/summarizer"
	"github.com/sneldao/sekats/internal/attest"
	"github.com/sneldao/sekats/internal/engine"
	"github.com/sneldao/sekats/internal/promotion"
	"github.com/sneldao/sekats/internal/review"
	"github.com/sneldao/sekats/internal/verdicts"
)

func fixtureDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Only copy the tracked fixture inputs — runtime state files
	// (verdicts, drafts, anchors) are gitignored and would leak a
	// developer's local server state into test fixtures.
	stateFiles := map[string]bool{
		"verdicts.json":           true,
		"pattern_rejections.json": true,
		"policy_drafts.json":      true,
		"attestations.json":       true,
	}
	files, err := filepath.Glob("../../data/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("fixture files: %v", err)
	}
	for _, path := range files {
		if stateFiles[filepath.Base(path)] {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(path)), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func pipelineApp(t *testing.T, dir string, ad adapters.Adapter) *app {
	t.Helper()
	ds, err := newDataSource(dir, ad)
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := parseTemplates("../../templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	promos := promotion.NewStore(
		filepath.Join(ds.stateDir, "pattern_rejections.json"),
		filepath.Join(ds.stateDir, "policy_drafts.json"), ad.NewDraft,
	)
	if err := promos.Load(); err != nil {
		t.Fatal(err)
	}
	return &app{
		tmpl: tmpl, dataDir: dir, adapter: ad, data: ds, vStore: ds.verdicts,
		promos: promos, promoteThreshold: 3, attests: attest.NewStore(""),
		// No anchorer is constructed: approval and docs are strictly offline.
	}
}

func request(t *testing.T, handler http.HandlerFunc, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	handler(w, httptest.NewRequest(method, path, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("%s %s: status %d: %s", method, path, w.Code, w.Body.String())
	}
	return w
}

// Both domains exercise identical handlers, queue/verdict durability, pattern
// thresholds, patch creation, and byte-stable attestation docs without signing.
func TestSharedAdapterPipeline(t *testing.T) {
	t.Setenv("SEKATS_DB", "off")
	for _, name := range []string{poker.Name, summarizer.Name} {
		t.Run(name, func(t *testing.T) {
			ad, err := adapters.Get(name)
			if err != nil {
				t.Fatal(err)
			}
			dir := fixtureDir(t)
			a := pipelineApp(t, dir, ad)
			st, surface, src, rep, trend, queue := a.data.Snapshot()
			if rep.TotalDecisions < 50 || len(rep.RulesRun) == 0 {
				t.Fatalf("incomplete decision/rule ingestion: %+v", rep)
			}
			if surface.AgreementTotal != st.Quality.AgreementRate.Total ||
				surface.LinkedReviews != st.Quality.LinkedReviews ||
				trend.AgreementAgree != surface.AgreementAgree ||
				trend.AgreementRatePct != surface.AgreementRatePct {
				t.Fatal("quality and trends disagree with ingested review counts")
			}
			if len(trend.Events) == 0 || trend.ReviewsWithTimestamps != 0 {
				t.Fatal("suggestion timeline is absent or review timestamps were invented")
			}
			home := request(t, a.handleHome, "GET", "/").Body.String()
			if !strings.Contains(home, ad.Agent()) ||
				!strings.Contains(home, fmt.Sprintf("%.1f", surface.AgreementRatePct)) {
				t.Fatal("quality surface missing adapter identity or agreement")
			}
			policyPage := request(t, a.handlePolicy, "GET", "/policy").Body.String()
			if !strings.Contains(policyPage, ad.PolicyFile()) {
				t.Fatal("policy page selected the wrong policy")
			}
			reviewPage := request(t, a.handleReview, "GET", "/review").Body.String()
			if !strings.Contains(reviewPage, `data-testid="review-item"`) {
				t.Fatal("empty review page")
			}
			trendsPage := request(t, a.handleTrends, "GET", "/trends").Body.String()
			if name == summarizer.Name {
				if !src.Synthetic || surface.AgreementTotal != 60 || surface.AgreementAgree != 24 ||
					len(rep.Violations) != 36 || surface.PolicySuggestionsTotal != 4 ||
					surface.ErrorReviews != 0 {
					t.Fatalf("unexpected summarizer stats: %+v, violations=%d", surface, len(rep.Violations))
				}
				for _, page := range []string{home, policyPage, reviewPage, trendsPage} {
					if strings.Contains(page, "poker-policy.json") || strings.Contains(page, "<th>Pot</th>") ||
						strings.Contains(page, "tools/sync_live.sh") ||
						strings.Contains(page, "bet vs check") || strings.Contains(page, "84.3%") {
						t.Fatal("poker assumptions leaked into summarizer UI")
					}
				}
				if !strings.Contains(home, "SYNTHETIC") || !strings.Contains(reviewPage, "source_text") ||
					!strings.Contains(reviewPage, "Agent rationale:") || !strings.Contains(reviewPage, "Outcome:") {
					t.Fatal("synthetic provenance or decision evidence missing")
				}
			}

			// Find a real repeated pattern in either domain, rather than hard-code
			// poker streets or summarizer input classes into the shared test.
			groups := make(map[string][]review.Item)
			for _, item := range queue.All() {
				key := ad.PatternKey(item.Review())
				groups[key] = append(groups[key], item)
			}
			var evidence []review.Item
			for _, item := range queue.All() {
				group := groups[ad.PatternKey(item.Review())]
				if len(group) >= 3 {
					evidence = group[:3]
					break
				}
			}
			if len(evidence) != 3 {
				t.Fatal("fixture has no repeated disagreement pattern")
			}
			for i, item := range evidence {
				response := request(t, a.handleReviewAction, "POST", fmt.Sprintf("/review/%d/reject", item.DecisionID))
				if !strings.Contains(response.Body.String(), `data-verdict="rejected"`) {
					t.Fatal("verdict partial not rendered")
				}
				if i < 2 && len(a.promos.AllDrafts()) != 0 {
					t.Fatal("draft staged before threshold")
				}
			}
			request(t, a.handleReviewAction, "POST", fmt.Sprintf("/review/%d/reject", evidence[0].DecisionID))
			drafts := a.promos.AllDrafts()
			if len(drafts) != 1 || len(drafts[0].Evidence) != 3 ||
				drafts[0].Status != engine.DraftPending || drafts[0].Target != ad.PolicyTarget() {
				t.Fatalf("threshold/deduplication/target: %+v", drafts)
			}
			var acceptedID int64
			for _, item := range queue.All() {
				if item.Verdict == review.VerdictPending {
					acceptedID = item.DecisionID
					break
				}
			}
			if acceptedID == 0 {
				t.Fatal("fixture needs a separate accept case")
			}
			request(t, a.handleReviewAction, "POST", fmt.Sprintf("/review/%d/accept", acceptedID))
			report := request(t, a.handleVerdicts, "GET", "/verdicts").Body.String()
			if !strings.Contains(report, "Adjudications") || !strings.Contains(report, "accepted on") ||
				!strings.Contains(report, "rejected on") {
				t.Fatal("human verdict report missing adjudications")
			}
			markdown, missing := verdicts.BuildReport(st.Quality.DisagreementsSample, a.vStore.All(), ad.Labels())
			if len(missing) != 0 || !strings.Contains(markdown, evidence[0].Review().ContextCode) {
				t.Fatal("verdicts did not join to their source decisions")
			}
			export := request(t, a.handleExport, "GET", "/export?download=true")
			var exported struct {
				Accepted int              `json:"accepted"`
				Rejected int              `json:"rejected"`
				Items    []exportItemJSON `json:"items"`
			}
			if err := json.Unmarshal(export.Body.Bytes(), &exported); err != nil {
				t.Fatal(err)
			}
			if exported.Accepted != 1 || exported.Rejected != 3 || len(exported.Items) != len(queue.All()) {
				t.Fatalf("export counts: %+v", exported)
			}
			if name == summarizer.Name {
				for _, item := range exported.Items {
					if item.ContextCode == "" || item.Phase == "" || len(item.Inputs) == 0 ||
						item.AgentRationale == "" || item.Outcome == "" || item.HandCode != "" || item.Pot != nil {
						t.Fatalf("generic evidence not exported: %+v", item)
					}
				}
			}

			draftID := drafts[0].ID
			pendingDoc := httptest.NewRecorder()
			a.handlePromotionAction(pendingDoc, httptest.NewRequest("GET", "/promotions/"+draftID+"/attestation", nil))
			if pendingDoc.Code != http.StatusNotFound {
				t.Fatal("unapproved draft exposed an attestation")
			}
			request(t, a.handlePromotionAction, "POST", "/promotions/"+draftID+"/approve")
			page := request(t, a.handlePromotions, "GET", "/promotions").Body.String()
			if !strings.Contains(page, ad.PolicyTarget()) ||
				!strings.Contains(page, "/path/to/"+ad.PolicyFile()) ||
				!strings.Contains(page, `data-testid="attest-off"`) {
				t.Fatal("approved card has wrong target, application path, or signing state")
			}
			if name == summarizer.Name && strings.Contains(page, "<th>Pot</th>") {
				t.Fatal("poker metric leaked into draft evidence")
			}
			patchResponse := request(t, a.handlePromotionAction, "GET", "/promotions/"+draftID+"/patch")
			var patch engine.Patch
			if err := json.Unmarshal(patchResponse.Body.Bytes(), &patch); err != nil {
				t.Fatal(err)
			}
			if patch.Target != ad.PolicyTarget() || len(patch.SourceDecisions) != 3 || len(patch.Entry) == 0 {
				t.Fatalf("invalid patch: %+v", patch)
			}
			docResponse := request(t, a.handlePromotionAction, "GET", "/promotions/"+draftID+"/attestation")
			var doc attest.Doc
			if err := json.Unmarshal(docResponse.Body.Bytes(), &doc); err != nil {
				t.Fatal(err)
			}
			if doc.Target != patch.Target || len(doc.SourceDecisions) != 3 ||
				docResponse.Header().Get("X-Sekats-Doc-SHA256") != fmt.Sprintf("%x", attest.DocHash(docResponse.Body.Bytes())) {
				t.Fatal("attestation provenance or digest mismatch")
			}

			restarted := pipelineApp(t, dir, ad)
			_, _, _, _, _, restoredQueue := restarted.data.Snapshot()
			_, accepted, rejected := restoredQueue.Counts()
			if accepted != 1 || rejected != 3 {
				t.Fatalf("verdicts lost on restart: accepted=%d rejected=%d", accepted, rejected)
			}
			restoredDrafts := restarted.promos.AllDrafts()
			if len(restoredDrafts) != 1 || restoredDrafts[0].Status != engine.DraftApproved {
				t.Fatal("draft lost on restart")
			}
			restoredDoc := request(t, restarted.handlePromotionAction, "GET", "/promotions/"+draftID+"/attestation")
			if !bytes.Equal(docResponse.Body.Bytes(), restoredDoc.Body.Bytes()) {
				t.Fatal("attestation doc changed across restart")
			}
		})
	}
}

func TestAdapterStateAndLiveIsolation(t *testing.T) {
	dir := fixtureDir(t)
	t.Setenv("SEKATS_DB", "off")
	p := pipelineApp(t, dir, poker.New())
	if err := p.vStore.Record(1, "rejected"); err != nil {
		t.Fatal(err)
	}
	if p.data.stateDir != dir {
		t.Fatal("poker state paths changed")
	}
	s := pipelineApp(t, dir, summarizer.New())
	if len(s.vStore.All()) != 0 || s.data.stateDir == p.data.stateDir {
		t.Fatal("poker verdicts leaked into summarizer")
	}
	request(t, s.handleReviewAction, "POST", "/review/1/accept")
	if len(p.vStore.All()) != 1 || p.vStore.All()[0].Verdict != "rejected" {
		t.Fatal("summarizer overwrote poker verdicts")
	}
	if err := os.MkdirAll(filepath.Join(dir, "live"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "live", "arena-poker.sqlite3"), []byte("not a database"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SEKATS_DB", "")
	ds, err := newDataSource(dir, summarizer.New())
	if err != nil || ds.livePath != "" || !ds.src.Synthetic {
		t.Fatalf("summarizer attempted poker autodetection: %v", err)
	}
	t.Setenv("SEKATS_DB", filepath.Join(dir, "live", "arena-poker.sqlite3"))
	if _, err := newDataSource(dir, summarizer.New()); err == nil {
		t.Fatal("fixture-only adapter accepted a poker live DB")
	}
}

// Nansen off + no jury contract: the enrichment APIs answer honestly —
// "unconfigured"/"disabled" sources, never fabricated labels.
func TestJuryAndLabelAPIsWhenDisabled(t *testing.T) {
	a := pipelineApp(t, fixtureDir(t), poker.New())
	w := request(t, a.handleJury, "GET", "/api/jury")
	var jury map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &jury); err != nil {
		t.Fatal(err)
	}
	if jury["source"] != "unconfigured" {
		t.Fatalf("jury source = %v, want unconfigured", jury["source"])
	}

	w = request(t, a.handleWalletLabels, "GET", "/api/wallet-labels?address=0x260e6103d23aab166c56417f3ba28af4239b348b")
	var labels map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &labels); err != nil {
		t.Fatal(err)
	}
	if labels["source"] != "disabled" {
		t.Fatalf("labels source = %v, want disabled", labels["source"])
	}
	// Rejects non-GET.
	w = httptest.NewRecorder()
	a.handleWalletLabels(w, httptest.NewRequest("POST", "/api/wallet-labels", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", w.Code)
	}
}

func TestJuryVerdictMarkupGatedByContract(t *testing.T) {
	off := pipelineApp(t, fixtureDir(t), poker.New())
	w := request(t, off.handleReview, "GET", "/review")
	if strings.Contains(w.Body.String(), "data-jury-vote") {
		t.Fatal("jury buttons rendered without a configured contract")
	}
	on := pipelineApp(t, fixtureDir(t), poker.New())
	on.juryContract = "0x00000000000000000000000000000000000000aa"
	on.publicRPC = "https://rpc.monad.xyz"
	on.attestExplorer = "https://monadscan.com"
	t.Setenv("SEKATS_CHAIN_ID", "")
	w = request(t, on.handleReview, "GET", "/review")
	body := w.Body.String()
	for _, want := range []string{"data-jury-vote=\"1\"", "data-jury-vote=\"2\"", "jury.js", "data-jury-contract", "data-jury-chain=\"143\"", "jury-tally"} {
		if !strings.Contains(body, want) {
			t.Fatalf("review page missing %q", want)
		}
	}
	// The htmx-swapped row after a verdict keeps its jury wiring.
	_, _, _, _, _, queue := on.data.Snapshot()
	id := queue.All()[0].DecisionID
	w = request(t, on.handleReviewAction, "POST", fmt.Sprintf("/review/%d/accept", id))
	if !strings.Contains(w.Body.String(), "data-jury-key") {
		t.Fatal("swapped row lost jury key")
	}
}
