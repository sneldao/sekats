package attest

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/sneldao/sekats/internal/engine"
	"golang.org/x/crypto/sha3"
)

const DomainName = "SekatsAttestations"
const DomainVersion = "2"

// ApprovalDoc excludes a future approval timestamp: fetching signing data never
// approves a draft, and the preimage stays identical after approval and retries.
type ApprovalDoc struct {
	Schema          string         `json:"schema"`
	DraftID         string         `json:"draft_id"`
	PatternKey      string         `json:"pattern_key"`
	Target          string         `json:"target"`
	Entry           map[string]any `json:"entry"`
	SourceDecisions []int64        `json:"source_decisions"`
	CreatedAt       time.Time      `json:"created_at"`
}

func ApprovalDocument(d engine.Draft) ([]byte, error) {
	if d.Status != engine.DraftPending && d.Status != engine.DraftApproved {
		return nil, fmt.Errorf("draft is not available for approval")
	}
	// Reuse the evidence ordering of the legacy document, not its timestamp.
	copy := d
	copy.Status = engine.DraftApproved
	copy.ResolvedAt = &d.CreatedAt
	doc, err := DocFor(copy)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(ApprovalDoc{
		Schema: "sekats-approval/v2", DraftID: d.ID, PatternKey: d.PatternKey,
		Target: d.Target, Entry: d.Payload, SourceDecisions: doc.SourceDecisions,
		CreatedAt: d.CreatedAt.UTC(),
	}, "", "  ")
}

type SigningConfig struct {
	ChainID   uint64
	Contract  string
	Approvers []string
	RPC       string
	Explorer  string
}

type TypedField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type TypedData struct {
	Types       map[string][]TypedField `json:"types"`
	PrimaryType string                  `json:"primaryType"`
	Domain      struct {
		Name              string `json:"name"`
		Version           string `json:"version"`
		ChainID           string `json:"chainId"`
		VerifyingContract string `json:"verifyingContract"`
	} `json:"domain"`
	Message struct {
		DraftKey string `json:"draftKey"`
		DocHash  string `json:"docHash"`
	} `json:"message"`
}

func Address(s string) (string, error) {
	if len(s) != 42 || !strings.HasPrefix(s, "0x") {
		return "", fmt.Errorf("invalid address")
	}
	raw, err := hex.DecodeString(s[2:])
	if err != nil || len(raw) != 20 || s == "0x0000000000000000000000000000000000000000" {
		return "", fmt.Errorf("invalid or zero address")
	}
	return strings.ToLower(s), nil
}

func (c SigningConfig) Validate() error {
	if c.ChainID == 0 {
		return fmt.Errorf("chain ID must be positive")
	}
	if _, err := Address(c.Contract); err != nil {
		return fmt.Errorf("contract: %w", err)
	}
	if len(c.Approvers) == 0 {
		return fmt.Errorf("SEKATS_APPROVERS must list authorized wallet addresses")
	}
	for _, addr := range c.Approvers {
		if _, err := Address(addr); err != nil {
			return fmt.Errorf("approver: %w", err)
		}
	}
	return nil
}

func (c SigningConfig) TypedData(key, hash [32]byte) TypedData {
	t := TypedData{
		PrimaryType: "Approval",
		Types: map[string][]TypedField{
			"EIP712Domain": {
				{"name", "string"}, {"version", "string"},
				{"chainId", "uint256"}, {"verifyingContract", "address"},
			},
			"Approval": {{"draftKey", "bytes32"}, {"docHash", "bytes32"}},
		},
	}
	t.Domain.Name, t.Domain.Version = DomainName, DomainVersion
	t.Domain.ChainID = strconv.FormatUint(c.ChainID, 10)
	t.Domain.VerifyingContract = c.Contract
	t.Message.DraftKey, t.Message.DocHash = Hex32(key), Hex32(hash)
	return t
}

func Hex32(hash [32]byte) string { return "0x" + hex.EncodeToString(hash[:]) }

func keccak(b []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(b)
	return h.Sum(nil)
}

// ApprovalDigest is keccak256(0x1901 || domainSeparator || structHash).
// Keep this encoding in sync with approvalDigest() in the contract.
func (c SigningConfig) ApprovalDigest(key, hash [32]byte) ([32]byte, error) {
	var result [32]byte
	if err := c.Validate(); err != nil {
		return result, err
	}
	domain := append([]byte{}, keccak([]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"))...)
	domain = append(domain, keccak([]byte(DomainName))...)
	domain = append(domain, keccak([]byte(DomainVersion))...)
	chain := make([]byte, 32)
	for i, n := 31, c.ChainID; n > 0; i, n = i-1, n>>8 {
		chain[i] = byte(n)
	}
	domain = append(domain, chain...)
	address, _ := hex.DecodeString(c.Contract[2:])
	domain = append(domain, make([]byte, 12)...)
	domain = append(domain, address...)
	body := append([]byte{}, keccak([]byte("Approval(bytes32 draftKey,bytes32 docHash)"))...)
	body = append(body, key[:]...)
	body = append(body, hash[:]...)
	digest := append([]byte{0x19, 0x01}, keccak(domain)...)
	digest = append(digest, keccak(body)...)
	copy(result[:], keccak(digest))
	return result, nil
}

// VerifySignature accepts only canonical low-S EOA signatures, as the contract
// does. No transaction or local approval is attempted before this check.
func (c SigningConfig) VerifySignature(key, hash [32]byte, signature string) (string, error) {
	digest, err := c.ApprovalDigest(key, hash)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(signature, "0x") || len(signature) != 132 {
		return "", fmt.Errorf("signature must be 65 hex bytes")
	}
	sig, err := hex.DecodeString(signature[2:])
	if err != nil {
		return "", fmt.Errorf("invalid signature hex")
	}
	v := sig[64]
	if v >= 27 {
		v -= 27
	}
	if v > 1 {
		return "", fmt.Errorf("invalid signature recovery ID")
	}
	var r, s secp256k1.ModNScalar
	if r.SetByteSlice(sig[:32]) || s.SetByteSlice(sig[32:64]) ||
		r.IsZero() || s.IsZero() || s.IsOverHalfOrder() {
		return "", fmt.Errorf("non-canonical signature")
	}
	compact := append([]byte{27 + v}, sig[:64]...)
	pub, _, err := ecdsa.RecoverCompact(compact, digest[:])
	if err != nil {
		return "", fmt.Errorf("invalid signature")
	}
	address := "0x" + hex.EncodeToString(keccak(pub.SerializeUncompressed()[1:])[12:])
	for _, allowed := range c.Approvers {
		if strings.EqualFold(address, allowed) {
			return address, nil
		}
	}
	return "", fmt.Errorf("signer is not a configured approver")
}
