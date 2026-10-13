package monad

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testAddress = "0x1234567890123456789012345678901234567890"

type walletRequest struct {
	JSONRPC string   `json:"jsonrpc"`
	ID      int      `json:"id"`
	Method  string   `json:"method"`
	Params  []string `json:"params"`
}

func walletFake(t *testing.T, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request walletRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" ||
			request.JSONRPC != "2.0" || len(request.Params) != 2 ||
			request.Params[0] != testAddress || request.Params[1] != "latest" {
			t.Errorf("request = %+v", request)
		}
		result := "0xde0b6b3a7640001"
		switch request.Method {
		case "eth_getBalance":
		case "eth_getTransactionCount":
			result = "0x20000000000001"
		default:
			t.Errorf("unexpected method %s", request.Method)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":%q}`, request.ID, result)
	}))
}

func TestWalletReadsExactQuantities(t *testing.T) {
	var calls atomic.Int32
	server := walletFake(t, &calls)
	defer server.Close()
	balance, count, err := NewClient(server.URL).Wallet(context.Background(), testAddress)
	if err != nil || balance != "1.000000000000000001" || count != "9007199254740993" || calls.Load() != 2 {
		t.Fatalf("wallet = %s, %s, %v; calls %d", balance, count, err, calls.Load())
	}
	for _, tc := range []struct{ wei, mon string }{
		{"0", "0"}, {"1", "0.000000000000000001"}, {"1000000000000000000", "1"},
		{"12500000000000000000", "12.5"}, {"123456789012345678901234567890", "123456789012.34567890123456789"},
	} {
		wei, _ := new(big.Int).SetString(tc.wei, 10)
		if got := formatMON(wei); got != tc.mon {
			t.Errorf("formatMON(%s) = %s, want %s", tc.wei, got, tc.mon)
		}
	}
}

func TestWalletRejectsRPCFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   string
		status int
	}{
		{"http", `{}`, 503},
		{"json", `not json`, 200},
		{"rpc error", `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"no"}}`, 200},
		{"wrong id", `{"jsonrpc":"2.0","id":2,"result":"0x1"}`, 200},
		{"wrong version", `{"jsonrpc":"1.0","id":1,"result":"0x1"}`, 200},
		{"missing result", `{"jsonrpc":"2.0","id":1}`, 200},
		{"empty quantity", `{"jsonrpc":"2.0","id":1,"result":"0x"}`, 200},
		{"negative quantity", `{"jsonrpc":"2.0","id":1,"result":"0x-1"}`, 200},
		{"invalid hex", `{"jsonrpc":"2.0","id":1,"result":"0xgg"}`, 200},
		{"leading zero", `{"jsonrpc":"2.0","id":1,"result":"0x01"}`, 200},
		{"oversized", strings.Repeat("x", (64<<10)+1), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			if _, _, err := NewClient(server.URL).Wallet(context.Background(), testAddress); err == nil {
				t.Fatal("expected RPC error")
			}
		})
	}
}

func TestWalletTimeoutAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer server.Close()
	client := NewClient(server.URL)
	client.http.Timeout = 20 * time.Millisecond
	start := time.Now()
	if _, _, err := client.Wallet(context.Background(), testAddress); err == nil {
		t.Fatal("expected timeout")
	}
	if time.Since(start) > time.Second {
		t.Fatal("read was not bounded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := client.Wallet(ctx, testAddress); err == nil {
		t.Fatal("expected cancellation")
	}
}

func TestTreasuryCacheAndExpiry(t *testing.T) {
	var calls atomic.Int32
	server := walletFake(t, &calls)
	defer server.Close()
	reader := NewTreasuryReader(server.URL, testAddress)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if state := reader.Snapshot(context.Background()); state.Status != "ready" || state.CheckedAt.IsZero() {
				t.Errorf("state = %+v", state)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 2 {
		t.Fatalf("cached reads called RPC %d times", calls.Load())
	}
	reader.expires = time.Time{}
	reader.Snapshot(context.Background())
	if calls.Load() != 4 {
		t.Fatalf("expired reads called RPC %d times", calls.Load())
	}
}

func TestTreasuryUnavailableAndUnconfigured(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	reader := NewTreasuryReader(server.URL, testAddress)
	for range 2 {
		state := reader.Snapshot(context.Background())
		if state.Status != "RPC unavailable" || state.Balance != "" || state.TxCount != "" {
			t.Fatalf("unavailable = %+v", state)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("failure cache calls = %d", calls.Load())
	}
	for _, reader := range []*TreasuryReader{
		nil, NewTreasuryReader(server.URL, ""), NewTreasuryReader(server.URL, "invalid"),
		NewTreasuryReader("file:///tmp/not-rpc", testAddress),
	} {
		if state := reader.Snapshot(context.Background()); state.Status != "not configured" {
			t.Fatalf("unconfigured = %+v", state)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("unconfigured treasury contacted RPC")
	}
}
