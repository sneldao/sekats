// Package monad reads public wallet state without signing or sending transactions.
package monad

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	rpcTimeout = 3 * time.Second
	cacheTTL   = 15 * time.Second
)

func ValidAddress(address string) bool {
	if len(address) != 42 || !strings.HasPrefix(address, "0x") {
		return false
	}
	_, err := hex.DecodeString(address[2:])
	return err == nil
}

func ValidRPC(endpoint string) bool {
	u, err := url.Parse(endpoint)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.Fragment == ""
}

type Client struct {
	endpoint string
	http     *http.Client
}

func NewClient(endpoint string) *Client {
	return &Client{endpoint: endpoint, http: &http.Client{Timeout: rpcTimeout}}
}

// Wallet reads only the two public Ethereum wallet getters, at latest.
func (c *Client) Wallet(ctx context.Context, address string) (balanceMON, txCount string, err error) {
	if !ValidAddress(address) || !ValidRPC(c.endpoint) {
		return "", "", errors.New("invalid wallet read configuration")
	}
	balance, err := c.quantity(ctx, "eth_getBalance", address, 1)
	if err != nil {
		return "", "", err
	}
	count, err := c.quantity(ctx, "eth_getTransactionCount", address, 2)
	if err != nil {
		return "", "", err
	}
	return formatMON(balance), count.String(), nil
}

func (c *Client) quantity(ctx context.Context, method, address string, id int) (*big.Int, error) {
	body, err := json.Marshal(struct {
		JSONRPC string   `json:"jsonrpc"`
		ID      int      `json:"id"`
		Method  string   `json:"method"`
		Params  []string `json:"params"`
	}{"2.0", id, method, []string{address, "latest"}})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("invalid RPC request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New("RPC request unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("RPC HTTP failure")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 {
		return nil, errors.New("RPC response unavailable")
	}
	var result struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  string          `json:"result"`
		Error   json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, errors.New("invalid RPC response")
	}
	if result.JSONRPC != "2.0" || result.ID != id || (len(result.Error) != 0 && string(result.Error) != "null") {
		return nil, errors.New("RPC response failure")
	}
	q := result.Result
	if !strings.HasPrefix(q, "0x") || len(q) < 3 || len(q) > 66 || (len(q) > 3 && q[2] == '0') {
		return nil, errors.New("invalid RPC quantity")
	}
	for _, ch := range q[2:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", ch) {
			return nil, errors.New("invalid RPC quantity")
		}
	}
	value, ok := new(big.Int).SetString(q[2:], 16)
	if !ok {
		return nil, errors.New("invalid RPC quantity")
	}
	return value, nil
}

func formatMON(wei *big.Int) string {
	unit := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	whole, fraction := new(big.Int), new(big.Int)
	whole.QuoRem(wei, unit, fraction)
	if fraction.Sign() == 0 {
		return whole.String()
	}
	decimal := fraction.String()
	decimal = strings.Repeat("0", 18-len(decimal)) + decimal
	return whole.String() + "." + strings.TrimRight(decimal, "0")
}

type Treasury struct {
	Status    string
	Address   string
	Balance   string
	TxCount   string
	CheckedAt time.Time
}

// TreasuryReader caches successes and failures briefly to bound RPC traffic.
type TreasuryReader struct {
	mu      sync.Mutex
	client  *Client
	address string
	cached  Treasury
	expires time.Time
}

func NewTreasuryReader(endpoint, address string) *TreasuryReader {
	return &TreasuryReader{client: NewClient(endpoint), address: address}
}

func (t *TreasuryReader) Snapshot(ctx context.Context) Treasury {
	if t == nil || !ValidAddress(t.address) || !ValidRPC(t.client.endpoint) {
		return Treasury{Status: "not configured"}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if time.Now().Before(t.expires) {
		return t.cached
	}
	ctx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
	balance, count, err := t.client.Wallet(ctx, t.address)
	state := Treasury{Status: "RPC unavailable", Address: t.address, CheckedAt: time.Now().UTC()}
	if err == nil {
		state.Status, state.Balance, state.TxCount = "ready", balance, count
	}
	t.cached, t.expires = state, time.Now().Add(cacheTTL)
	return state
}
