// Package enrich attaches Nansen Profiler wallet labels to the addresses
// the console already displays — the agent treasury, anchor attesters, and
// permissionless jurors. Labels are optional: without SEKATS_NANSEN_API_KEY
// the client serves a bundled snapshot (or nothing) and the UI stays quiet,
// matching the LIVE/FROZEN badge discipline — never imply a live feed we
// do not have.
package enrich

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	labelsURL     = "https://api.nansen.ai/api/v1/profiler/address/labels"
	requestTimout = 5 * time.Second
	cacheTTL      = 24 * time.Hour
)

// Label is one Nansen wallet classification.
type Label struct {
	Label    string   `json:"label"`
	Category string   `json:"category,omitempty"`
	Kind     []string `json:"kind,omitempty"`
}

// Entry is the cached/snapshot record for one address.
type Entry struct {
	Labels []Label   `json:"labels"`
	AsOf   time.Time `json:"as_of"`
}

// Result is what the API surface returns per address.
type Result struct {
	Address string  `json:"address"`
	Labels  []Label `json:"labels"`
	Source  string  `json:"source"` // "nansen" | "snapshot" | "disabled"
	AsOf    string  `json:"as_of,omitempty"`
}

type Client struct {
	apiKey   string
	endpoint string // production labelsURL; tests may point it at httptest
	http     *http.Client
	snapshot map[string]Entry // lowercased address → entry, from disk

	mu    sync.Mutex
	cache map[string]Entry
}

// New returns a client; apiKey empty means live calls are disabled and only
// the snapshot file (dataDir/nansen-labels.json) can answer.
func New(apiKey, dataDir string) *Client {
	c := &Client{
		apiKey:   strings.TrimSpace(apiKey),
		endpoint: labelsURL,
		http:     &http.Client{Timeout: requestTimout},
		snapshot: map[string]Entry{},
		cache:    map[string]Entry{},
	}
	if dataDir == "" {
		return c
	}
	raw, err := os.ReadFile(filepath.Join(dataDir, "nansen-labels.json"))
	if err != nil {
		return c
	}
	var file map[string]Entry
	if json.Unmarshal(raw, &file) == nil {
		for addr, e := range file {
			c.snapshot[strings.ToLower(addr)] = e
		}
	}
	return c
}

// Live reports whether real Nansen calls are configured.
func (c *Client) Live() bool { return c != nil && c.apiKey != "" }

// Lookup returns labels for one address. Order: fresh cache → live Nansen →
// snapshot → disabled. A live fetch failure still serves a snapshot if one
// exists; otherwise the caller gets "unavailable" and should render nothing.
func (c *Client) Lookup(ctx context.Context, address string) Result {
	res := Result{Address: address, Source: "disabled"}
	if c == nil || !validAddress(address) {
		return res
	}
	key := strings.ToLower(address)

	c.mu.Lock()
	if e, ok := c.cache[key]; ok && time.Since(e.AsOf) < cacheTTL {
		c.mu.Unlock()
		return Result{Address: address, Labels: e.Labels, Source: "nansen", AsOf: e.AsOf.UTC().Format(time.RFC3339)}
	}
	c.mu.Unlock()

	if c.apiKey != "" {
		if labels, err := c.fetch(ctx, address); err == nil {
			e := Entry{Labels: labels, AsOf: time.Now().UTC()}
			c.mu.Lock()
			c.cache[key] = e
			c.mu.Unlock()
			return Result{Address: address, Labels: labels, Source: "nansen", AsOf: e.AsOf.Format(time.RFC3339)}
		}
	}
	if e, ok := c.snapshot[key]; ok {
		return Result{Address: address, Labels: e.Labels, Source: "snapshot", AsOf: e.AsOf.UTC().Format(time.RFC3339)}
	}
	return res
}

func (c *Client) fetch(ctx context.Context, address string) ([]Label, error) {
	body, _ := json.Marshal(map[string]any{
		"address":    address,
		"chain":      "monad",
		"pagination": map[string]int{"page": 1, "per_page": 20},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("nansen: " + resp.Status)
	}
	var out struct {
		Data []Label `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func validAddress(a string) bool {
	if len(a) != 42 || !strings.HasPrefix(a, "0x") {
		return false
	}
	for _, ch := range a[2:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", ch) {
			return false
		}
	}
	return true
}
