package enrich

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testAddr = "0x260e6103d23aab166c56417f3ba28af4239b348b"

func TestLookupDisabledNoKeyNoSnapshot(t *testing.T) {
	c := New("", t.TempDir())
	res := c.Lookup(context.Background(), testAddr)
	if res.Source != "disabled" || len(res.Labels) != 0 {
		t.Fatalf("expected disabled/empty, got %+v", res)
	}
	if c.Live() {
		t.Fatal("Live() true without a key")
	}
}

func TestLookupRejectsBadAddress(t *testing.T) {
	c := New("key", "")
	res := c.Lookup(context.Background(), "not-an-address")
	if res.Source != "disabled" {
		t.Fatalf("bad address should not reach upstream: %+v", res)
	}
	res = c.Lookup(context.Background(), "")
	if res.Source != "disabled" {
		t.Fatalf("empty address should not reach upstream: %+v", res)
	}
}

func TestLookupLiveSuccess(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("apikey") != "test-key" {
			t.Errorf("apikey header missing: %v", r.Header)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["chain"] != "monad" {
			t.Errorf("chain = %v, want monad", body["chain"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"label": "Smart Money", "category": "smart_money", "kind": []string{"fund"}},
		}})
	}))
	defer srv.Close()

	c := New("test-key", "")
	c.endpoint = srv.URL

	res := c.Lookup(context.Background(), testAddr)
	if res.Source != "nansen" || len(res.Labels) != 1 || res.Labels[0].Label != "Smart Money" {
		t.Fatalf("unexpected result: %+v", res)
	}
	// Second call must be served from cache — no second HTTP request.
	res = c.Lookup(context.Background(), testAddr)
	if res.Source != "nansen" || len(res.Labels) != 1 {
		t.Fatalf("cached result changed: %+v", res)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected 1 upstream call, got %d", got)
	}
}

func TestLookupLiveErrorFallsToSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	dir := t.TempDir()
	snap := map[string]Entry{
		testAddr: {Labels: []Label{{Label: "Fund"}}, AsOf: time.Now().UTC()},
	}
	raw, _ := json.Marshal(snap)
	if err := os.WriteFile(filepath.Join(dir, "nansen-labels.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	c := New("test-key", dir)
	c.endpoint = srv.URL
	res := c.Lookup(context.Background(), testAddr)
	if res.Source != "snapshot" || len(res.Labels) != 1 || res.Labels[0].Label != "Fund" {
		t.Fatalf("snapshot fallback failed: %+v", res)
	}
}

func TestLookupMalformedBodyAndUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	c := New("key", "")
	c.endpoint = srv.URL
	res := c.Lookup(context.Background(), testAddr)
	if res.Source != "disabled" || len(res.Labels) != 0 {
		t.Fatalf("malformed body should yield disabled/empty: %+v", res)
	}
	srv.Close()

	c.endpoint = "http://127.0.0.1:1/unreachable"
	res = c.Lookup(context.Background(), testAddr)
	if res.Source != "disabled" || len(res.Labels) != 0 {
		t.Fatalf("unreachable host should yield disabled/empty: %+v", res)
	}
}

func TestLookupSnapshotOnlyWithoutKey(t *testing.T) {
	dir := t.TempDir()
	snap := map[string]Entry{
		strings.ToUpper(testAddr[:8]) + testAddr[8:]: {Labels: []Label{{Label: "Exchange"}}, AsOf: time.Now().UTC()},
	}
	raw, _ := json.Marshal(snap)
	os.WriteFile(filepath.Join(dir, "nansen-labels.json"), raw, 0o600)

	c := New("", dir)
	res := c.Lookup(context.Background(), testAddr)
	if res.Source != "snapshot" || len(res.Labels) != 1 {
		t.Fatalf("mixed-case snapshot key lookup failed: %+v", res)
	}
}
