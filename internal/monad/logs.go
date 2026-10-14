package monad

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Log is one eth_getLogs record.
type Log struct {
	Address     string   `json:"address"`
	Topics      []string `json:"topics"`
	Data        string   `json:"data"`
	BlockNumber string   `json:"blockNumber"`
	Index       string   `json:"logIndex"`
	TxHash      string   `json:"transactionHash"`
}

// BlockNumber returns the chain's latest block height.
func (c *Client) BlockNumber(ctx context.Context) (uint64, error) {
	var out string
	if err := c.call(ctx, "eth_blockNumber", nil, &out); err != nil {
		return 0, err
	}
	return parseQuantity(out)
}

// DefaultLogChunk is the eth_getLogs window the client chunks ranges into.
// Public Monad RPCs cap the range around 100 blocks; indexer-grade
// endpoints (Envio, Alchemy) allow far more — widen via SetLogChunk.
const DefaultLogChunk = 100

// SetLogChunk overrides the per-call eth_getLogs block window.
func (c *Client) SetLogChunk(n uint64) {
	if n > 0 {
		c.logChunk = n
	}
}

// LogChunk reports the effective eth_getLogs window.
func (c *Client) LogChunk() uint64 {
	if c.logChunk == 0 {
		return DefaultLogChunk
	}
	return c.logChunk
}

// Logs fetches eth_getLogs for one contract+topic0 in [from, to], chunking
// the range so public-RPC range limits do not sink the call.
func (c *Client) Logs(ctx context.Context, contract, topic0 string, from, to uint64) ([]Log, error) {
	if to < from {
		return nil, nil
	}
	window := c.logChunk
	if window == 0 {
		window = DefaultLogChunk
	}
	var all []Log
	for start := from; start <= to; {
		end := start + window - 1
		if end > to {
			end = to
		}
		var chunk []Log
		err := c.call(ctx, "eth_getLogs", []any{map[string]any{
			"address":   contract,
			"topics":    []string{topic0},
			"fromBlock": "0x" + strconv.FormatUint(start, 16),
			"toBlock":   "0x" + strconv.FormatUint(end, 16),
		}}, &chunk)
		if err != nil {
			return all, fmt.Errorf("getLogs %d-%d: %w", start, end, err)
		}
		all = append(all, chunk...)
		if end == to {
			break
		}
		start = end + 1
	}
	return all, nil
}

func (c *Client) call(ctx context.Context, method string, params any, out any) error {
	body, err := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
	}{"2.0", 1, method, paramsOrDefault(params)})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("RPC request unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("RPC HTTP failure")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 {
		return errors.New("RPC response unavailable")
	}
	var result struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return errors.New("invalid RPC response")
	}
	if len(result.Error) != 0 && string(result.Error) != "null" {
		return fmt.Errorf("RPC error: %s", truncate(string(result.Error), 200))
	}
	return json.Unmarshal(result.Result, out)
}

func paramsOrDefault(p any) any {
	if p == nil {
		return []string{}
	}
	return p
}

func parseQuantity(q string) (uint64, error) {
	q = strings.TrimPrefix(q, "0x")
	return strconv.ParseUint(q, 16, 64)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
