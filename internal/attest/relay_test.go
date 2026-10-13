package attest

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

// All "cast" commands below are local shell fixtures, never Foundry transactions.
func fixtureCast(t *testing.T, script string) *CastAnch {
	t.Helper()
	path := t.TempDir() + "/cast"
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := vectorConfig()
	return &CastAnch{
		Bin: path, Key: "not-a-key-test-marker", ContractAddr: cfg.Contract,
		Config: cfg, RPC: "http://example.invalid",
	}
}

func TestRelayVerifiesHashAndHumanSigner(t *testing.T) {
	key, hash := DraftKey("vector"), DocHash([]byte("canonical document"))
	script := fmt.Sprintf(`
case "$1" in
  chain-id) echo 143 ;;
  send)
    [ "$ETH_PRIVATE_KEY" = "not-a-key-test-marker" ] || exit 2
    for arg in "$@"; do [ "$arg" != "--private-key" ] || exit 3; done
    [ "$3" = "anchorWithSig(bytes32,bytes32,bytes)" ] || exit 4
    printf '{"transactionHash":"0xfixture","from":"0xrelayer","status":"0x1"}'
    ;;
  call)
    case "$3" in
      'anchored(bytes32)(bytes32)') echo '%s' ;;
      'attesters(bytes32)(address)') echo '%s' ;;
      *) exit 5 ;;
    esac
    ;;
  *) exit 6 ;;
esac
`, Hex32(hash), fixtureSigner)
	c := fixtureCast(t, script)
	tx, err := c.AnchorWithSig(context.Background(), key, hash, fixtureSignature)
	if err != nil || tx != "0xfixture" {
		t.Fatalf("relay: %s %v", tx, err)
	}
	c = fixtureCast(t, strings.Replace(script, fixtureSigner, "0x3333333333333333333333333333333333333333", 1))
	if _, err := c.AnchorWithSig(context.Background(), key, hash, fixtureSignature); err == nil {
		t.Fatal("relayer accepted as human signer")
	}
	c = fixtureCast(t, strings.Replace(script, Hex32(hash), Hex32(DocHash([]byte("wrong"))), 1))
	if _, err := c.AnchorWithSig(context.Background(), key, hash, fixtureSignature); err == nil {
		t.Fatal("hash mismatch accepted")
	}
}

func TestRelayWrongChainDoesNotSendAndSanitizesFailures(t *testing.T) {
	c := fixtureCast(t, `
if [ "$1" = chain-id ]; then echo 1; exit 0; fi
echo "must never send" >&2
exit 99
`)
	_, err := c.AnchorWithSig(context.Background(), DraftKey("vector"), DocHash([]byte("canonical document")), fixtureSignature)
	if err == nil || !strings.Contains(err.Error(), "chain") {
		t.Fatal("wrong RPC chain accepted")
	}
	c = fixtureCast(t, `echo "not-a-key-test-marker" >&2; exit 1`)
	_, err = c.send(context.Background(), "fixture()")
	if err == nil || strings.Contains(err.Error(), "not-a-key-test-marker") {
		t.Fatal("subprocess stderr exposed")
	}
}

func TestPolicyRelayReadsAppendIndex(t *testing.T) {
	hash := DocHash([]byte("resulting policy file"))
	c := fixtureCast(t, fmt.Sprintf(`
case "$1" in
  chain-id) echo 143 ;;
  send)
    [ "$3" = "anchorPolicy(bytes32,bytes32)" ] || exit 2
    printf '{"transactionHash":"0xfixture","from":"0xoperator","status":"0x1"}'
    ;;
  call)
    case "$3" in
      'policyCount(bytes32)(uint256)') echo '2' ;;
      'policyHistory(bytes32,uint256)(bytes32)')
        [ "$5" = 2 ] || exit 3
        echo '%s'
        ;;
      *) exit 4 ;;
    esac
    ;;
  *) exit 5 ;;
esac
`, Hex32(hash)))
	tx, operator, err := c.AnchorPolicy(context.Background(), DraftKey("vector"), hash)
	if err != nil || tx != "0xfixture" || operator != "0xoperator" {
		t.Fatalf("policy relay: %s %s %v", tx, operator, err)
	}
}

func TestStoreWriteFailureDoesNotCommitMemory(t *testing.T) {
	s := NewStore(t.TempDir() + "/missing/records.json")
	if err := s.Put(Record{DraftID: "fixture", Status: "anchored"}); err == nil {
		t.Fatal("expected write failure")
	}
	if _, exists := s.For("fixture"); exists {
		t.Fatal("failed durable write mutated memory")
	}
}

func TestStorePolicySlicesAreIsolated(t *testing.T) {
	s := NewStore("")
	r := Record{DraftID: "fixture", Policies: []PolicyRecord{{PolicySHA256: "original"}}}
	s.Put(r)
	r.Policies[0].PolicySHA256 = "input mutation"
	got, _ := s.For("fixture")
	if got.Policies[0].PolicySHA256 != "original" {
		t.Fatal("store retained caller's slice")
	}
	got.Policies[0].PolicySHA256 = "output mutation"
	got, _ = s.For("fixture")
	if got.Policies[0].PolicySHA256 != "original" {
		t.Fatal("store exposed internal slice")
	}
}
