package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sneldao/sekats/internal/attest"
	"github.com/sneldao/sekats/internal/engine"
	"github.com/sneldao/sekats/internal/monad"
	"github.com/sneldao/sekats/internal/promotion"
)

const contractAddress = "0x1234567890123456789012345678901234567890"

func registryApp(t *testing.T) *app {
	t.Helper()
	tmpl, err := parseTemplates(filepath.Join("..", "..", "templates", "*.html"))
	if err != nil {
		t.Fatal(err)
	}
	return &app{
		tmpl: tmpl, attests: attest.NewStore(""), promos: promotion.NewStore("", "", nil),
		data: &dataSource{}, attestExplorer: "https://monadscan.com",
		publicRPC: "https://rpc.monad.xyz", attestContract: contractAddress,
	}
}

func TestAnchorsHandlersUsePersistedStoreWithoutSigner(t *testing.T) {
	a := registryApp(t)
	path := filepath.Join(t.TempDir(), "attestations.json")
	a.attests = attest.NewStore(path)
	for _, rec := range []attest.Record{
		{DraftID: "orphan", DocSHA256: strings.Repeat("a", 64), Status: "anchored", Contract: contractAddress, Attester: contractAddress, TxHash: "0xabc"},
		{DraftID: "legacy", DocSHA256: strings.Repeat("b", 64), Status: "anchored"},
		{DraftID: "failed", Status: "failed"},
	} {
		if err := a.attests.Put(rec); err != nil {
			t.Fatal(err)
		}
	}
	a.attests = attest.NewStore(path)
	if err := a.attests.Load(); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/anchors", a.handleAnchors)
	mux.HandleFunc("/api/anchors", a.handleAnchorsJSON)
	server := httptest.NewServer(mux)
	defer server.Close()
	resp, err := http.Get(server.URL + "/api/anchors")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var registry anchorRegistry
	if err := json.NewDecoder(resp.Body).Decode(&registry); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "no-store" ||
		len(registry.Anchors) != 2 || registry.Anchors[0].DraftID != "legacy" {
		t.Fatalf("response %d registry = %+v", resp.StatusCode, registry)
	}
	row := registry.Anchors[1]
	if row.DraftKey != fmt.Sprintf("0x%x", attest.DraftKey("orphan")) ||
		row.DocHash != "0x"+strings.Repeat("a", 64) ||
		row.Attester != contractAddress || row.TxHash != "0xabc" ||
		row.TxURL != "https://monadscan.com/tx/0xabc" ||
		row.DocURL != "/promotions/orphan/attestation" ||
		row.CastCall != "cast call "+contractAddress+` "anchored(bytes32)(bytes32)" `+row.DraftKey+" --rpc-url 'https://rpc.monad.xyz'" {
		t.Fatalf("row = %+v", row)
	}
	if registry.Anchors[0].Contract != contractAddress {
		t.Fatal("legacy contract fallback missing")
	}
	w := httptest.NewRecorder()
	a.handleAnchors(w, httptest.NewRequest("GET", "/anchors", nil))
	for _, want := range []string{"Anchors Registry", "orphan", "data-rpc", "anchors.js", "Copy cast call", "0xabc"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("HTML missing %q", want)
		}
	}
	if w.Code != 200 || strings.Contains(w.Body.String(), ">failed<") {
		t.Fatalf("HTML status = %d", w.Code)
	}
}

func TestAnchorsEmptyAndReadOnly(t *testing.T) {
	a := registryApp(t)
	for _, path := range []string{"/anchors", "/api/anchors"} {
		handler := a.handleAnchors
		if path == "/api/anchors" {
			handler = a.handleAnchorsJSON
		}
		for _, method := range []string{"POST", "PUT", "DELETE"} {
			w := httptest.NewRecorder()
			handler(w, httptest.NewRequest(method, path, nil))
			if w.Code != 405 || w.Header().Get("Allow") != "GET, HEAD" {
				t.Fatalf("%s %s = %d", method, path, w.Code)
			}
		}
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest("GET", path+"/bad", nil))
		if w.Code != 404 {
			t.Fatalf("unexpected path = %d", w.Code)
		}
		w = httptest.NewRecorder()
		handler(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("empty registry = %d", w.Code)
		}
		if path == "/api/anchors" && !strings.Contains(w.Body.String(), `"anchors":[]`) {
			t.Fatal("empty JSON anchors must be an array")
		}
		if path == "/anchors" && !strings.Contains(w.Body.String(), "No anchored drafts") {
			t.Fatal("missing honest empty state")
		}
	}
}

func TestRegistryEscapesHTMLAndShell(t *testing.T) {
	a := registryApp(t)
	a.publicRPC = "https://rpc.example/?q=';echo"
	if err := a.attests.Put(attest.Record{DraftID: `<script>alert(1)</script>`, Status: "anchored"}); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	a.handleAnchors(w, httptest.NewRequest("GET", "/anchors", nil))
	if strings.Contains(w.Body.String(), "<script>alert(1)</script>") || w.Code != 200 {
		t.Fatal("draft ID was not safely escaped")
	}
	command := a.anchorRegistry(context.Background()).Anchors[0].CastCall
	if !strings.Contains(command, `'https://rpc.example/?q='\'';echo'`) {
		t.Fatalf("unsafe shell command %s", command)
	}
	a.attestContract = ""
	if a.anchorRegistry(context.Background()).Anchors[0].CastCall != "" {
		t.Fatal("unconfigured contract produced a cast call")
	}
}

func TestCanonicalAttestationBytesRemainAvailable(t *testing.T) {
	a := registryApp(t)
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	draft := engine.Draft{ID: "approved", Status: engine.DraftApproved, ResolvedAt: &now, Payload: map[string]any{"rule": "check"}}
	raw, err := json.Marshal(map[string]engine.Draft{draft.ID: draft})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "drafts.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	a.promos = promotion.NewStore("", path, nil)
	if err := a.promos.Load(); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	a.handlePromotionAction(w, httptest.NewRequest("GET", "/promotions/approved/attestation", nil))
	doc, err := attest.DocFor(draft)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := doc.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || w.Body.String() != string(canonical) {
		t.Fatalf("canonical doc response = %d %s", w.Code, w.Body.String())
	}
}

func TestQualitySurfaceTreasuryStates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		result := "0xde0b6b3a7640000"
		if req.Method == "eth_getTransactionCount" {
			result = "0x2"
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":%q}`, req.ID, result)
	}))
	defer server.Close()
	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer unavailable.Close()
	for _, tc := range []struct {
		name   string
		reader *monad.TreasuryReader
		want   string
	}{
		{"missing", nil, "not configured"},
		{"live", monad.NewTreasuryReader(server.URL, contractAddress), "the agent you're supervising holds <strong>1 MON, 2 txs</strong>"},
		{"failed", monad.NewTreasuryReader(unavailable.URL, contractAddress), "RPC unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := registryApp(t)
			a.treasury = tc.reader
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil).WithContext(context.Background())
			a.handleHome(w, r)
			if w.Code != 200 || !strings.Contains(w.Body.String(), tc.want) {
				t.Fatalf("home = %d; missing %q", w.Code, tc.want)
			}
		})
	}
}
