package monad

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// rpcStub answers JSON-RPC with canned results keyed by method.
type rpcStub struct {
	results map[string]any
	saw     [][]any // decoded params arrays
}

func (s *rpcStub) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("bad request body: %v", err)
			return
		}
		var params []any
		if err := json.Unmarshal(req.Params, &params); err != nil {
			t.Errorf("params for %s not an array (JSON-RPC spec): %v", req.Method, err)
		}
		s.saw = append(s.saw, params)
		res, ok := s.results[req.Method]
		if !ok {
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32601}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": res})
	}
}

func TestBlockNumber(t *testing.T) {
	stub := &rpcStub{results: map[string]any{"eth_blockNumber": "0x2a"}}
	srv := httptest.NewServer(stub.handler(t))
	defer srv.Close()

	n, err := NewClient(srv.URL).BlockNumber(context.Background())
	if err != nil || n != 42 {
		t.Fatalf("BlockNumber = %d, %v; want 42, nil", n, err)
	}
}

func TestLogsParamsShapeAndChunks(t *testing.T) {
	stub := &rpcStub{results: map[string]any{"eth_getLogs": []Log{}}}
	srv := httptest.NewServer(stub.handler(t))
	defer srv.Close()

	// 250-block span → three 100-block windows (2 full + 1 partial).
	logs, err := NewClient(srv.URL).Logs(context.Background(), "0x0a87", "0xtopic", 0, 249)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected empty log set, got %d", len(logs))
	}
	if len(stub.saw) != 3 {
		t.Fatalf("expected 3 chunked calls, got %d", len(stub.saw))
	}
	filter, ok := stub.saw[0][0].(map[string]any)
	if !ok {
		t.Fatalf("eth_getLogs param 0 not a filter object: %#v", stub.saw[0])
	}
	if filter["address"] != "0x0a87" || filter["fromBlock"] != "0x0" || filter["toBlock"] != "0x63" {
		t.Fatalf("bad first filter: %v", filter)
	}
	last := stub.saw[2][0].(map[string]any)
	if last["fromBlock"] != "0xc8" || last["toBlock"] != "0xf9" {
		t.Fatalf("bad final window: %v", last)
	}
}

func TestLogsHonoursSetLogChunk(t *testing.T) {
	stub := &rpcStub{results: map[string]any{"eth_getLogs": []Log{}}}
	srv := httptest.NewServer(stub.handler(t))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetLogChunk(10000)
	if _, err := c.Logs(context.Background(), "0x0a87", "0xtopic", 0, 20000); err != nil {
		t.Fatalf("Logs: %v", err)
	}
	if len(stub.saw) != 3 { // 0-9999, 10000-19999, 20000-20000
		t.Fatalf("expected 3 chunked calls at 10k window, got %d", len(stub.saw))
	}
}

func TestLogsRPCErrorPropagates(t *testing.T) {
	stub := &rpcStub{results: map[string]any{}} // no result → RPC error object
	srv := httptest.NewServer(stub.handler(t))
	defer srv.Close()

	if _, err := NewClient(srv.URL).Logs(context.Background(), "0x0a87", "0xtopic", 0, 10); err == nil {
		t.Fatal("expected error when RPC returns an error object")
	}
}

func TestLogsEmptyRange(t *testing.T) {
	stub := &rpcStub{results: map[string]any{"eth_getLogs": []Log{}}}
	srv := httptest.NewServer(stub.handler(t))
	defer srv.Close()

	logs, err := NewClient(srv.URL).Logs(context.Background(), "0x0a87", "0xtopic", 10, 5)
	if err != nil || logs != nil || len(stub.saw) != 0 {
		t.Fatalf("to<from should return nil without RPC calls: %v %v", logs, err)
	}
}
