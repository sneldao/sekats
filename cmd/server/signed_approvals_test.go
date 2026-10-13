package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/sneldao/sekats/internal/adapters/poker"
	"github.com/sneldao/sekats/internal/attest"
	"github.com/sneldao/sekats/internal/engine"
	"github.com/sneldao/sekats/internal/promotion"
	"golang.org/x/crypto/sha3"
)

const publicSignature = "0x79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f8179800000000000000000000000000000000000000000000000000000000000000011b"

type fakeSignedAnchorer struct {
	cfg           attest.SigningConfig
	approvalCalls int
	policyCalls   int
	gotDoc        [32]byte
	gotPolicy     [32]byte
	relayError    error
}

func (f *fakeSignedAnchorer) Anchor(context.Context, [32]byte, [32]byte) (string, string, error) {
	panic("legacy path must not be used in signed mode")
}
func (f *fakeSignedAnchorer) ChainName() string { return "Monad (chain 143)" }
func (f *fakeSignedAnchorer) Contract() string  { return f.cfg.Contract }
func (f *fakeSignedAnchorer) ExplorerTX(tx string) string {
	return "https://example.invalid/tx/" + tx
}
func (f *fakeSignedAnchorer) SigningConfig() attest.SigningConfig { return f.cfg }
func (f *fakeSignedAnchorer) AnchorWithSig(_ context.Context, _ [32]byte, hash [32]byte, _ string) (string, error) {
	f.approvalCalls++
	f.gotDoc = hash
	return "0xapproval", f.relayError
}
func (f *fakeSignedAnchorer) AnchorPolicy(_ context.Context, _ [32]byte, hash [32]byte) (string, string, error) {
	f.policyCalls++
	f.gotPolicy = hash
	return "0xpolicy", "0xoperator", f.relayError
}

func testApprovalApp(t *testing.T) (*app, *fakeSignedAnchorer) {
	t.Helper()
	store := promotion.NewStore("", "", func(key string, ev []engine.Evidence) engine.Draft {
		return engine.Draft{
			ID: "vector", PatternKey: key, Target: "postflop.promotedFromHandReviews",
			Payload: map[string]any{"rule": "test rule"}, Evidence: ev,
			CreatedAt: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC),
		}
	})
	d := store.RecordRejection("pattern", engine.Evidence{DecisionID: 1}, 1)
	cfg := attest.SigningConfig{
		ChainID: 143, Contract: "0x1111111111111111111111111111111111111111",
		Approvers: []string{"0x2222222222222222222222222222222222222222"},
	}
	doc, err := attest.ApprovalDocument(*d)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := cfg.ApprovalDigest(attest.DraftKey(d.ID), attest.DocHash(doc))
	if err != nil {
		t.Fatal(err)
	}
	// Recover fixed public signature components for this fixture's document.
	// No private key is created or used.
	raw, _ := hex.DecodeString(publicSignature[2:])
	compact := append([]byte{27}, raw[:64]...)
	pub, _, err := ecdsa.RecoverCompact(compact, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	h := sha3.NewLegacyKeccak256()
	h.Write(pub.SerializeUncompressed()[1:])
	cfg.Approvers = []string{"0x" + hex.EncodeToString(h.Sum(nil)[12:])}
	fake := &fakeSignedAnchorer{cfg: cfg}
	a := &app{promos: store, attests: attest.NewStore(""), anchorer: fake}
	return a, fake
}

func approvalRequest(a *app, method, action, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(method, "/promotions/vector/"+action, strings.NewReader(body))
	a.handlePromotionAction(rec, r)
	return rec
}

func submitApproval(a *app) *httptest.ResponseRecorder {
	return approvalRequest(a, http.MethodPost, "signature", `{"signature":"`+publicSignature+`"}`)
}

func TestTypedDataDoesNotApproveAndUsesCanonicalDocument(t *testing.T) {
	a, f := testApprovalApp(t)
	rec := approvalRequest(a, http.MethodGet, "typed-data", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Document string           `json:"document"`
		Typed    attest.TypedData `json:"typed_data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &response)
	d, _ := a.promos.Draft("vector")
	doc, _ := attest.ApprovalDocument(d)
	if d.Status != engine.DraftPending || d.ResolvedAt != nil || response.Document != string(doc) ||
		response.Typed.Message.DocHash != attest.Hex32(attest.DocHash(doc)) || f.approvalCalls != 0 {
		t.Fatal("fetch changed state or returned a non-canonical document")
	}
}

func TestSignatureApprovalAndDoubleSubmission(t *testing.T) {
	a, f := testApprovalApp(t)
	d, _ := a.promos.Draft("vector")
	doc, _ := attest.ApprovalDocument(d)
	rec := submitApproval(a)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d: %s", rec.Code, rec.Body.String())
	}
	d, _ = a.promos.Draft("vector")
	record, ok := a.attests.For("vector")
	if !ok || d.Status != engine.DraftApproved || record.Status != "anchored" ||
		record.Attester != f.cfg.Approvers[0] || f.gotDoc != attest.DocHash(doc) {
		t.Fatalf("wrong approval: %+v", record)
	}
	download := approvalRequest(a, http.MethodGet, "attestation", "")
	if download.Code != http.StatusOK || download.Body.String() != string(doc) {
		t.Fatal("downloaded preimage differs from signing document")
	}
	if submitApproval(a).Code != http.StatusConflict || f.approvalCalls != 1 {
		t.Fatal("duplicate submission relayed")
	}
}

func TestApprovalRejectsUntrustedInputsWithoutStateChange(t *testing.T) {
	for _, name := range []string{"hash", "malformed", "trailing", "signature", "approver", "chain", "contract", "changed-doc", "discarded"} {
		t.Run(name, func(t *testing.T) {
			a, f := testApprovalApp(t)
			body := `{"signature":"` + publicSignature + `"}`
			switch name {
			case "hash":
				body = `{"signature":"` + publicSignature + `","docHash":"0xjunk"}`
			case "malformed":
				body = "{"
			case "trailing":
				body += "{}"
			case "signature":
				body = `{"signature":"0x00"}`
			case "approver":
				f.cfg.Approvers = []string{"0x3333333333333333333333333333333333333333"}
			case "chain":
				f.cfg.ChainID++
			case "contract":
				f.cfg.Contract = "0x3333333333333333333333333333333333333333"
			case "changed-doc":
				d, _ := a.promos.Draft("vector")
				d.Payload["rule"] = "tampered"
			case "discarded":
				a.promos.Discard("vector")
			}
			rec := approvalRequest(a, http.MethodPost, "signature", body)
			if rec.Code < 400 || f.approvalCalls != 0 {
				t.Fatalf("input accepted: %d %s", rec.Code, rec.Body.String())
			}
			d, _ := a.promos.Draft("vector")
			if d.Status == engine.DraftApproved {
				t.Fatal("invalid signature approved draft")
			}
		})
	}
}

func TestRelayFailureCanRetrySameDocument(t *testing.T) {
	a, f := testApprovalApp(t)
	f.relayError = errors.New("fixture relay failed")
	if rec := submitApproval(a); rec.Code != http.StatusBadGateway {
		t.Fatal(rec.Body.String())
	}
	record, _ := a.attests.For("vector")
	if record.Status != "failed" || record.Signature != publicSignature {
		t.Fatal("failed relay lost signed approval")
	}
	first := f.gotDoc
	f.relayError = nil
	if rec := submitApproval(a); rec.Code != http.StatusOK || f.gotDoc != first {
		t.Fatalf("retry changed preimage: %s", rec.Body.String())
	}
}

func TestSignedModeBlocksLegacyApprovalAndCrossOrigin(t *testing.T) {
	a, f := testApprovalApp(t)
	for _, action := range []string{"approve", "attest"} {
		if approvalRequest(a, http.MethodPost, action, "").Code != http.StatusConflict {
			t.Fatal("legacy path was enabled")
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/promotions/vector/signature", strings.NewReader(`{"signature":"`+publicSignature+`"}`))
	r.Header.Set("Origin", "https://attacker.invalid")
	rec := httptest.NewRecorder()
	a.handlePromotionAction(rec, r)
	if rec.Code != http.StatusForbidden || f.approvalCalls != 0 {
		t.Fatal("cross-origin relay accepted")
	}
}

func TestAppliedAnchorsServerReadFileAndRequiresApproval(t *testing.T) {
	a, f := testApprovalApp(t)
	if approvalRequest(a, http.MethodPost, "applied", "{}").Code != http.StatusConflict {
		t.Fatal("policy anchored before approval")
	}
	submitApproval(a)
	a.policyFile = t.TempDir() + "/policy.json"
	if err := os.WriteFile(a.policyFile, []byte(`{"postflop":{"promotedFromHandReviews":[]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if approvalRequest(a, http.MethodPost, "applied", "{}").Code != http.StatusConflict || f.policyCalls != 0 {
		t.Fatal("unapplied policy accepted")
	}
	raw := []byte("{\"postflop\":{\"promotedFromHandReviews\":[{\"rule\":\"test rule\"}]}}\n")
	os.WriteFile(a.policyFile, raw, 0600)
	rec := approvalRequest(a, http.MethodPost, "applied", `{"policyHash":"ignored","path":"/never-read-client-path"}`)
	if rec.Code != http.StatusBadRequest || f.policyCalls != 0 {
		t.Fatal("client-supplied policy hash/path accepted")
	}
	rec = approvalRequest(a, http.MethodPost, "applied", "{}")
	if rec.Code != http.StatusOK || f.gotPolicy != attest.DocHash(raw) {
		t.Fatalf("policy not hashed from file: %d %s", rec.Code, rec.Body.String())
	}
	approvalRequest(a, http.MethodPost, "applied", "{}")
	if f.policyCalls != 1 {
		t.Fatal("identical applied bytes submitted twice")
	}
	raw = append(raw, '\n')
	os.WriteFile(a.policyFile, raw, 0600)
	approvalRequest(a, http.MethodPost, "applied", "{}")
	record, _ := a.attests.For("vector")
	if len(record.Policies) != 2 || f.policyCalls != 2 || f.gotPolicy != attest.DocHash(raw) {
		t.Fatal("policy lineage did not append exact new bytes")
	}
}

func TestPersistenceFailureStopsRelay(t *testing.T) {
	a, f := testApprovalApp(t)
	a.attests = attest.NewStore(t.TempDir() + "/nonexistent/attestations.json")
	if rec := submitApproval(a); rec.Code != http.StatusInternalServerError || f.approvalCalls != 0 {
		t.Fatal("transaction sent before durable signature record")
	}
}

func TestWalletApprovalTemplates(t *testing.T) {
	a, _ := testApprovalApp(t)
	a.adapter = poker.New()
	tmpl, err := template.ParseGlob("../../templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	a.tmpl = tmpl
	d, _ := a.promos.Draft("vector")
	var out bytes.Buffer
	if err := tmpl.ExecuteTemplate(&out, "promotion-card", a.card(d)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Approve with wallet") ||
		strings.Contains(out.String(), `hx-post="/promotions/vector/approve"`) {
		t.Fatal("signed card offers unsigned approval")
	}
	submitApproval(a)
	a.policyFile = "operator-policy-file"
	d, _ = a.promos.Draft("vector")
	out.Reset()
	if err := tmpl.ExecuteTemplate(&out, "promotion-card", a.card(d)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Mark applied and anchor policy") ||
		strings.Contains(out.String(), `hx-post="/promotions/vector/attest"`) {
		t.Fatal("approved signed card offers legacy relay or lacks policy reporting")
	}
	rec := httptest.NewRecorder()
	a.handlePromotions(rec, httptest.NewRequest(http.MethodGet, "/promotions", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "/static/js/wallet-approval.js") {
		t.Fatal("promotions page lacks wallet script")
	}
}
