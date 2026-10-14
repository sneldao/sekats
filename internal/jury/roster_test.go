package jury

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sneldao/sekats/internal/enrich"
	"github.com/sneldao/sekats/internal/monad"
)

const (
	jurorA = "0x0000000000000000000000000000000000000aa1"
	jurorB = "0x0000000000000000000000000000000000000bb2"
)

func topic(addr string) string {
	return "0x000000000000000000000000" + addr[2:]
}

func decisionKey(b byte) string {
	s := "0x"
	for i := 0; i < 64; i++ {
		s += "0"
	}
	return s[:64] + string([]byte{'0' + b%10}) + "f"
}

func verdictData(v byte) string {
	s := "0x"
	for i := 0; i < 63; i++ {
		s += "0"
	}
	return s + string([]byte{'0' + v})
}

// logRPC serves a fixed head and log list over JSON-RPC.
func logRPC(t *testing.T, head string, logs []monad.Log) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		var res any
		switch req.Method {
		case "eth_blockNumber":
			res = head
		case "eth_getLogs":
			res = logs
		default:
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32601}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": res})
	}))
}

func TestRosterDedupLatestWins(t *testing.T) {
	k1 := decisionKey(1)
	logs := []monad.Log{
		// jurorA flips 1 → 2 on key1 (later block/log wins).
		{Topics: []string{verdictEventTopic, k1, topic(jurorA)}, Data: verdictData(1), BlockNumber: "0xa", Index: "0x0"},
		{Topics: []string{verdictEventTopic, k1, topic(jurorA)}, Data: verdictData(2), BlockNumber: "0xa", Index: "0x1"},
		// jurorB votes agent on key1.
		{Topics: []string{verdictEventTopic, k1, topic(jurorB)}, Data: verdictData(1), BlockNumber: "0xa", Index: "0x2"},
		// malformed log — wrong topic count, must be skipped.
		{Topics: []string{verdictEventTopic, k1}, Data: verdictData(1), BlockNumber: "0xa", Index: "0x3"},
	}
	srv := logRPC(t, "0x100", logs)
	defer srv.Close()

	w := NewWatcher(monad.NewClient(srv.URL), "0x0a87c9a14e8a4b7b18b5e7d0d14f2f0d1171c9a1", 0, enrich.New("", ""), "")
	// Drive the indexer directly: two 100-block steps cover head (0x100).
	w.step(context.Background())
	w.step(context.Background())
	r := w.Snapshot(context.Background())
	if r.Source != "chain" {
		t.Fatalf("source = %q, want chain", r.Source)
	}
	if len(r.Jurors) != 2 {
		t.Fatalf("expected 2 jurors, got %+v", r.Jurors)
	}
	var a, b *Juror
	for i := range r.Jurors {
		switch r.Jurors[i].Address {
		case jurorA:
			a = &r.Jurors[i]
		case jurorB:
			b = &r.Jurors[i]
		}
	}
	if a == nil || a.AgentVotes != 0 || a.ShadowVotes != 1 {
		t.Fatalf("jurorA latest verdict should be shadow=1, agent=0: %+v", a)
	}
	if b == nil || b.AgentVotes != 1 || b.ShadowVotes != 0 {
		t.Fatalf("jurorB should be agent=1: %+v", b)
	}
	// Enricher disabled → no labels, but source stays honest.
	for _, j := range r.Jurors {
		if len(j.Labels) != 0 || j.LabelsSource != "" {
			t.Fatalf("labels should be empty with enricher off: %+v", j)
		}
	}
}

func TestRosterUnavailableWhenRPCDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	w := NewWatcher(monad.NewClient(srv.URL), "0x0a87c9a14e8a4b7b18b5e7d0d14f2f0d1171c9a1", 0, nil, "")
	w.step(context.Background())
	r := w.Snapshot(context.Background())
	if r.Source != "unavailable" || len(r.Jurors) != 0 {
		t.Fatalf("expected unavailable/empty, got %+v", r)
	}
}

func TestRosterUnconfiguredWithoutContract(t *testing.T) {
	w := NewWatcher(monad.NewClient("http://127.0.0.1:1"), "", 0, nil, "")
	r := w.Snapshot(context.Background())
	if r.Source != "unconfigured" {
		t.Fatalf("expected unconfigured, got %+v", r)
	}
}
