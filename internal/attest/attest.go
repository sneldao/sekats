// Package attest verifies wallet approvals and relays document and policy
// hashes on Monad. It also supports an explicitly configured legacy contract.
// It never writes agent policy files. Anchored lineage does not prove execution.
package attest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sneldao/sekats/internal/engine"
)

// Record is the durable outcome of anchoring one approved draft.
type Record struct {
	DraftID    string         `json:"draft_id"`
	DocSHA256  string         `json:"doc_sha256"` // sha256 hex of the canonical Doc
	Chain      string         `json:"chain"`
	Contract   string         `json:"contract,omitempty"`
	TxHash     string         `json:"tx_hash,omitempty"`
	Attester   string         `json:"attester,omitempty"`
	Status     string         `json:"status"` // "anchored" or "failed"
	Error      string         `json:"error,omitempty"`
	AnchoredAt *time.Time     `json:"anchored_at,omitempty"`
	Signature  string         `json:"signature,omitempty"`
	DocSchema  string         `json:"doc_schema,omitempty"`
	Policies   []PolicyRecord `json:"policies,omitempty"`
}

type PolicyRecord struct {
	PolicySHA256 string    `json:"policy_sha256"`
	TxHash       string    `json:"tx_hash,omitempty"`
	Attester     string    `json:"attester,omitempty"`
	Status       string    `json:"status"`
	Error        string    `json:"error,omitempty"`
	RecordedAt   time.Time `json:"recorded_at"`
}

// Doc is the canonical attestation payload — the preimage of the
// on-chain hash. It is byte-stable: every field is fixed at approval
// time and MarshalIndent emits map keys sorted, so re-serving the doc
// produces identical bytes and an identical digest.
type Doc struct {
	Schema          string         `json:"schema"` // "sekats-attestation/v1"
	DraftID         string         `json:"draft_id"`
	PatternKey      string         `json:"pattern_key"`
	Target          string         `json:"target"`
	Entry           map[string]any `json:"entry"`
	SourceDecisions []int64        `json:"source_decisions"`
	ApprovedAt      time.Time      `json:"approved_at"`
}

// DocFor builds the canonical attestation doc for an approved draft.
func DocFor(d engine.Draft) (Doc, error) {
	if d.Status != engine.DraftApproved || d.ResolvedAt == nil {
		return Doc{}, fmt.Errorf("draft %s is not approved", d.ID)
	}
	ids := make([]int64, 0, len(d.Evidence))
	for _, e := range d.Evidence {
		ids = append(ids, e.DecisionID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return Doc{
		Schema:          "sekats-attestation/v1",
		DraftID:         d.ID,
		PatternKey:      d.PatternKey,
		Target:          d.Target,
		Entry:           d.Payload,
		SourceDecisions: ids,
		ApprovedAt:      d.ResolvedAt.UTC(),
	}, nil
}

// CanonicalJSON renders the doc byte-stably.
func (d Doc) CanonicalJSON() ([]byte, error) {
	return json.MarshalIndent(d, "", "  ")
}

// DraftKey derives the contract's bytes32 key for a draft id.
// Domain-separated so an attacker can't pick a draft id that collides
// with some other application's key space.
func DraftKey(draftID string) [32]byte {
	return sha256.Sum256([]byte("sekats/draft/" + draftID))
}

// DecisionKey derives the SekatsJury contract's bytes32 key for a review
// decision id — the same domain-separation scheme as DraftKey.
func DecisionKey(decisionID int64) [32]byte {
	return sha256.Sum256([]byte("sekats/decision/" + strconv.FormatInt(decisionID, 10)))
}

// DocHash is the digest anchored on-chain.
func DocHash(doc []byte) [32]byte { return sha256.Sum256(doc) }

// Anchorer preserves the legacy transaction interface. SignedAnchorer extends
// it with locally verified human signatures and policy-file lineage.
type Anchorer interface {
	// Anchor writes docHash under draftKey; returns tx hash and attester.
	Anchor(ctx context.Context, draftKey, docHash [32]byte) (txHash, attester string, err error)
	// ChainName is the display label, e.g. "Monad (chain 143)".
	ChainName() string
	// Contract is the on-chain registry address.
	Contract() string
	// ExplorerTX renders an explorer URL for a tx, "" when no explorer
	// is configured.
	ExplorerTX(txHash string) string
}

type SignedAnchorer interface {
	Anchorer
	SigningConfig() SigningConfig
	AnchorWithSig(context.Context, [32]byte, [32]byte, string) (string, error)
	AnchorPolicy(context.Context, [32]byte, [32]byte) (string, string, error)
}

// Store persists one Record per draft, flushed on every write — same
// durability contract as the verdicts and promotion stores.
type Store struct {
	mu      sync.Mutex
	path    string
	records map[string]Record
}

// NewStore persists to path; an empty path keeps state in memory only.
func NewStore(path string) *Store {
	return &Store{path: path, records: map[string]Record{}}
}

// Load reads previously-saved records; a missing file means empty state.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		return nil
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(raw, &s.records)
}

// For returns the record for one draft.
func (s *Store) For(draftID string) (Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[draftID]
	r.Policies = append([]PolicyRecord(nil), r.Policies...)
	return r, ok
}

// AnchoredCount reports how many drafts carry a confirmed anchor.
func (s *Store) AnchoredCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.records {
		if r.Status == "anchored" {
			n++
		}
	}
	return n
}

// Put records the outcome durably.
func (s *Store) Put(r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.Policies = append([]PolicyRecord(nil), r.Policies...)
	if s.path == "" {
		s.records[r.DraftID] = r
		return nil
	}
	next := make(map[string]Record, len(s.records)+1)
	for id, record := range s.records {
		next[id] = record
	}
	next[r.DraftID] = r
	b, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	// Atomic replacement avoids a truncated approval record after a crash.
	f, err := os.CreateTemp(filepath.Dir(s.path), ".attestations-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(f.Name(), s.path); err != nil {
		return err
	}
	s.records = next
	return nil
}

// FromEnv resolves anchoring config. Returns nil (disabled) when
// SEKATS_ATTEST=off or when key/contract/cast are missing — the console
// still works, it just doesn't anchor.
func FromEnv() (Anchorer, error) {
	if os.Getenv("SEKATS_ATTEST") == "off" {
		return nil, nil
	}
	key := os.Getenv("SEKATS_ATTEST_KEY")
	contract := os.Getenv("SEKATS_ATTEST_CONTRACT")
	if key == "" || contract == "" {
		return nil, nil
	}
	bin := envOr("CAST_BIN", "cast")
	if _, err := exec.LookPath(bin); err != nil {
		return nil, fmt.Errorf("cast not found (%s): install Foundry to enable anchoring", bin)
	}
	legacy := os.Getenv("SEKATS_ATTEST_LEGACY") == "true"
	cfg := SigningConfig{
		Contract: contract, RPC: envOr("SEKATS_RPC", "https://rpc.monad.xyz"),
		Explorer: envOr("SEKATS_EXPLORER", "https://monadscan.com"),
	}
	chain, err := strconv.ParseUint(envOr("SEKATS_CHAIN_ID", "143"), 10, 64)
	if err != nil || chain == 0 {
		return nil, fmt.Errorf("invalid SEKATS_CHAIN_ID")
	}
	cfg.ChainID = chain
	for _, addr := range strings.Split(os.Getenv("SEKATS_APPROVERS"), ",") {
		if addr = strings.TrimSpace(addr); addr != "" {
			cfg.Approvers = append(cfg.Approvers, addr)
		}
	}
	if !legacy {
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
	}
	c := &CastAnch{
		RPC:          envOr("SEKATS_RPC", "https://rpc.monad.xyz"),
		ContractAddr: contract,
		Key:          key,
		Bin:          bin,
		Chain:        fmt.Sprintf("Monad (chain %d)", chain),
		Explorer:     envOr("SEKATS_EXPLORER", "https://monadscan.com"),
		Config:       cfg,
	}
	if legacy {
		// The wrapper intentionally does not implement SignedAnchorer.
		return legacyAnch{c}, nil
	}
	return c, nil
}

type legacyAnch struct{ c *CastAnch }

func (l legacyAnch) Anchor(ctx context.Context, key, hash [32]byte) (string, string, error) {
	return l.c.anchorLegacy(ctx, key, hash)
}
func (l legacyAnch) ChainName() string           { return l.c.ChainName() }
func (l legacyAnch) Contract() string            { return l.c.Contract() }
func (l legacyAnch) ExplorerTX(tx string) string { return l.c.ExplorerTX(tx) }

// CastAnch delegates relay signing to cast. Key material never appears in argv,
// persisted records, or returned subprocess output.
type CastAnch struct {
	RPC          string
	ContractAddr string
	Key          string
	Bin          string
	Chain        string
	Explorer     string
	Config       SigningConfig
}

func (c *CastAnch) ChainName() string { return c.Chain }
func (c *CastAnch) Contract() string  { return c.ContractAddr }
func (c *CastAnch) ExplorerTX(tx string) string {
	if c.Explorer == "" || tx == "" {
		return ""
	}
	return strings.TrimRight(c.Explorer, "/") + "/tx/" + tx
}

// castReceipt is the subset of `cast send --json` output we need.
type castReceipt struct {
	TransactionHash string `json:"transactionHash"`
	From            string `json:"from"`
	Status          string `json:"status"`
}

func (c *CastAnch) SigningConfig() SigningConfig { return c.Config }

// Anchor is deliberately unavailable for the redesigned contract.
func (c *CastAnch) Anchor(ctx context.Context, draftKey, docHash [32]byte) (string, string, error) {
	return "", "", fmt.Errorf("human wallet signature required")
}

func (c *CastAnch) send(ctx context.Context, method string, params ...string) (castReceipt, error) {
	args := append([]string{"send", c.ContractAddr, method}, params...)
	args = append(args, "--rpc-url", c.RPC, "--chain", strconv.FormatUint(c.Config.ChainID, 10), "--json")
	cmd := exec.CommandContext(ctx, c.Bin, args...)
	cmd.Env = append(os.Environ(), "ETH_PRIVATE_KEY="+c.Key)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return castReceipt{}, fmt.Errorf("cast send failed: %v", err)
	}
	var rc castReceipt
	if err := json.Unmarshal(stdout.Bytes(), &rc); err != nil {
		return rc, fmt.Errorf("invalid cast receipt")
	}
	if rc.Status != "0x1" || rc.TransactionHash == "" {
		return rc, fmt.Errorf("transaction reverted or receipt incomplete")
	}
	return rc, nil
}

func (c *CastAnch) anchorLegacy(ctx context.Context, draftKey, docHash [32]byte) (string, string, error) {
	rc, err := c.send(ctx, "anchor(bytes32,bytes32)", Hex32(draftKey), Hex32(docHash))
	if err != nil {
		return rc.TransactionHash, rc.From, err
	}
	// Read back anchored(draftKey) and compare. A mined tx is not proof:
	// the contract address could be wrong, empty, or on another chain —
	// the whole point of the feature is verification, so verify.
	if err := c.verifyOnChain(ctx, draftKey, docHash); err != nil {
		return rc.TransactionHash, rc.From, fmt.Errorf("tx %s mined but read-back failed: %v", rc.TransactionHash, err)
	}
	return rc.TransactionHash, rc.From, nil
}

func (c *CastAnch) checkChain(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, c.Bin, "chain-id", "--rpc-url", c.RPC).Output()
	if err != nil || strings.TrimSpace(string(out)) != strconv.FormatUint(c.Config.ChainID, 10) {
		return fmt.Errorf("RPC chain does not match signing domain")
	}
	return nil
}

func (c *CastAnch) call(ctx context.Context, method string, params ...string) (string, error) {
	args := append([]string{"call", c.ContractAddr, method}, params...)
	args = append(args, "--rpc-url", c.RPC)
	out, err := exec.CommandContext(ctx, c.Bin, args...).Output()
	if err != nil {
		return "", fmt.Errorf("contract read-back failed")
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *CastAnch) AnchorWithSig(ctx context.Context, key, hash [32]byte, sig string) (string, error) {
	signer, err := c.Config.VerifySignature(key, hash, sig)
	if err != nil {
		return "", err
	}
	if err := c.checkChain(ctx); err != nil {
		return "", err
	}
	rc, err := c.send(ctx, "anchorWithSig(bytes32,bytes32,bytes)", Hex32(key), Hex32(hash), sig)
	if err != nil {
		return rc.TransactionHash, err
	}
	if err := c.verifyOnChain(ctx, key, hash); err != nil {
		return rc.TransactionHash, err
	}
	got, err := c.call(ctx, "attesters(bytes32)(address)", Hex32(key))
	if err != nil || !strings.EqualFold(got, signer) {
		return rc.TransactionHash, fmt.Errorf("attester read-back does not match human signer")
	}
	return rc.TransactionHash, nil
}

func (c *CastAnch) AnchorPolicy(ctx context.Context, key, hash [32]byte) (string, string, error) {
	if err := c.checkChain(ctx); err != nil {
		return "", "", err
	}
	before, err := c.call(ctx, "policyCount(bytes32)(uint256)", Hex32(key))
	if err != nil {
		return "", "", err
	}
	// cast decorates large uint256 values with a decimal annotation.
	fields := strings.Fields(before)
	if len(fields) == 0 {
		return "", "", fmt.Errorf("invalid policy count")
	}
	if _, err := strconv.ParseUint(fields[0], 10, 64); err != nil {
		return "", "", fmt.Errorf("invalid policy count")
	}
	rc, err := c.send(ctx, "anchorPolicy(bytes32,bytes32)", Hex32(key), Hex32(hash))
	if err != nil {
		return rc.TransactionHash, rc.From, err
	}
	got, err := c.call(ctx, "policyHistory(bytes32,uint256)(bytes32)", Hex32(key), fields[0])
	if err != nil || !strings.EqualFold(got, Hex32(hash)) {
		return rc.TransactionHash, rc.From, fmt.Errorf("policy history read-back mismatch")
	}
	return rc.TransactionHash, rc.From, nil
}

// verifyOnChain calls anchored(draftKey) and requires it to equal docHash.
func (c *CastAnch) verifyOnChain(ctx context.Context, draftKey, docHash [32]byte) error {
	out, err := exec.CommandContext(ctx, c.Bin, "call", c.ContractAddr,
		"anchored(bytes32)(bytes32)",
		"0x"+hex.EncodeToString(draftKey[:]),
		"--rpc-url", c.RPC).Output()
	if err != nil {
		return err
	}
	want := "0x" + hex.EncodeToString(docHash[:])
	if strings.TrimSpace(string(out)) != want {
		return fmt.Errorf("anchored() returned %.66s, want %s", string(out), want)
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
