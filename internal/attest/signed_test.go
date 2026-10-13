package attest

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sneldao/sekats/internal/engine"
)

// Independent ethers 6 TypedDataEncoder/recoverAddress vector. Fixed public
// (r,s,v) components were recovered, not signed with a private key.
const fixtureSignature = "0x79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f8179800000000000000000000000000000000000000000000000000000000000000011b"
const fixtureSigner = "0x2213bbc63b52a099282ba34039bf75fd37c8a9ec"

func vectorConfig() SigningConfig {
	return SigningConfig{
		ChainID: 143, Contract: "0x1111111111111111111111111111111111111111",
		Approvers: []string{fixtureSigner},
	}
}

func TestTypedDataDigestIndependentVector(t *testing.T) {
	cfg := vectorConfig()
	key, hash := DraftKey("vector"), DocHash([]byte("canonical document"))
	digest, err := cfg.ApprovalDigest(key, hash)
	if err != nil {
		t.Fatal(err)
	}
	if Hex32(digest) != "0xf49fe2542718ca572b676f3e0c3525b4b19fd13696f56cbe683f5583feea3aa0" {
		t.Fatalf("wrong EIP-712 digest: %x", digest)
	}
	data := cfg.TypedData(key, hash)
	raw, _ := json.Marshal(data)
	if !strings.Contains(string(raw), `"chainId":"143"`) ||
		data.Domain.Name != DomainName || data.Domain.Version != "2" ||
		data.PrimaryType != "Approval" || data.Message.DocHash != Hex32(hash) {
		t.Fatalf("bad typed data: %s", raw)
	}
	signer, err := cfg.VerifySignature(key, hash, fixtureSignature)
	if err != nil || signer != fixtureSigner {
		t.Fatalf("recovery = %s, %v", signer, err)
	}
	// Wallets sometimes encode recovery ID as 0/1 instead of 27/28.
	if _, err := cfg.VerifySignature(key, hash, fixtureSignature[:130]+"00"); err != nil {
		t.Fatal(err)
	}
}

func TestSignatureRejections(t *testing.T) {
	key, hash := DraftKey("vector"), DocHash([]byte("canonical document"))
	for _, name := range []string{"chain", "contract", "key", "hash", "unauthorized", "length", "hex", "v", "zero-r", "zero-s", "high-s"} {
		t.Run(name, func(t *testing.T) {
			cfg, k, h, sig := vectorConfig(), key, hash, fixtureSignature
			switch name {
			case "chain":
				cfg.ChainID++
			case "contract":
				cfg.Contract = "0x2222222222222222222222222222222222222222"
			case "key":
				k = DraftKey("another-draft")
			case "hash":
				h = DocHash([]byte("tampered"))
			case "unauthorized":
				cfg.Approvers = []string{"0x2222222222222222222222222222222222222222"}
			case "length":
				sig = "0x00"
			case "hex":
				sig = "0xzz" + sig[4:]
			case "v":
				sig = sig[:130] + "02"
			case "zero-r":
				sig = "0x" + strings.Repeat("0", 64) + sig[66:]
			case "zero-s":
				sig = sig[:66] + strings.Repeat("0", 64) + sig[130:]
			case "high-s":
				sig = sig[:66] + "fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364140" + "1c"
			}
			if _, err := cfg.VerifySignature(k, h, sig); err == nil {
				t.Fatal("unexpected accepted signature")
			}
		})
	}
}

func TestApprovalDocumentStableAcrossApproval(t *testing.T) {
	d := approvedDraft()
	d.CreatedAt = time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	d.Status, d.ResolvedAt = engine.DraftPending, nil
	before, err := ApprovalDocument(d)
	if err != nil {
		t.Fatal(err)
	}
	d.Status = engine.DraftApproved
	now := time.Now()
	d.ResolvedAt = &now
	after, err := ApprovalDocument(d)
	if err != nil || string(before) != string(after) {
		t.Fatalf("approval changed preimage: %v", err)
	}
	if strings.Contains(string(after), "approved_at") {
		t.Fatal("signed document includes unknown future timestamp")
	}
	d.Status = engine.DraftDiscarded
	if _, err := ApprovalDocument(d); err == nil {
		t.Fatal("discarded draft accepted")
	}
}

func TestSigningConfigRejections(t *testing.T) {
	for _, name := range []string{"chain", "contract", "approvers", "approver"} {
		t.Run(name, func(t *testing.T) {
			cfg := vectorConfig()
			switch name {
			case "chain":
				cfg.ChainID = 0
			case "contract":
				cfg.Contract = "not-address"
			case "approvers":
				cfg.Approvers = nil
			case "approver":
				cfg.Approvers = []string{"0x0000000000000000000000000000000000000000"}
			}
			if _, err := cfg.ApprovalDigest([32]byte{}, [32]byte{}); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}

func TestAppliedPolicyHashExactBytes(t *testing.T) {
	d := approvedDraft()
	raw := []byte(`{"postflop":{"promotedFromHandReviews":[{"rule":"test rule","pattern":"flop: bet→check"}]}}`)
	hash, err := AppliedPolicyHash(raw, d)
	if err != nil || hash != DocHash(raw) {
		t.Fatalf("policy hash = %s, %v", hex.EncodeToString(hash[:]), err)
	}
	pretty := append(raw, '\n')
	second, err := AppliedPolicyHash(pretty, d)
	if err != nil || second == hash {
		t.Fatal("policy file bytes were canonicalized")
	}
	for _, bad := range []string{"not json", "{}", `{"postflop":{"promotedFromHandReviews":{}}}`, `{"postflop":{"promotedFromHandReviews":[{"rule":"wrong"}]}}`} {
		if _, err := AppliedPolicyHash([]byte(bad), d); err == nil {
			t.Fatalf("bad policy accepted: %s", bad)
		}
	}
}

func TestLegacyModeExplicit(t *testing.T) {
	t.Setenv("SEKATS_ATTEST", "")
	t.Setenv("SEKATS_ATTEST_KEY", "not-a-key-test-config-only")
	t.Setenv("SEKATS_ATTEST_CONTRACT", vectorConfig().Contract)
	t.Setenv("CAST_BIN", "/bin/true") // No subprocess is invoked.
	t.Setenv("SEKATS_CHAIN_ID", "143")
	t.Setenv("SEKATS_APPROVERS", fixtureSigner)
	t.Setenv("SEKATS_ATTEST_LEGACY", "")
	a, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := a.(SignedAnchorer); !ok {
		t.Fatal("signed mode not the default")
	}
	t.Setenv("SEKATS_ATTEST_LEGACY", "true")
	a, err = FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := a.(SignedAnchorer); ok {
		t.Fatal("legacy mode advertises signed approval support")
	}
}
