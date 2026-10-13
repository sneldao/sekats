package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sneldao/sekats/internal/attest"
	"github.com/sneldao/sekats/internal/engine"
)

func (a *app) signedAnchorer() attest.SignedAnchorer {
	s, _ := a.anchorer.(attest.SignedAnchorer)
	return s
}

func (a *app) approvalDocument(d engine.Draft) ([]byte, error) {
	rec, exists := a.attests.For(d.ID)
	if (exists && rec.DocSchema == "sekats-approval/v2") ||
		(!exists && a.signedAnchorer() != nil) {
		return attest.ApprovalDocument(d)
	}
	doc, err := attest.DocFor(d)
	if err != nil {
		return nil, err
	}
	return doc.CanonicalJSON()
}

func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

// Mutations accept only same-origin browser requests (or non-browser operator
// clients without Origin). This is not operator authentication; deploy behind
// access control, especially when enabling the policy-file reporting endpoint.
func sameOrigin(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		return origin == scheme+"://"+r.Host
	}
	return true
}

func (a *app) handleSignedApproval(w http.ResponseWriter, r *http.Request, id, action string) {
	expected := http.MethodPost
	if action == "typed-data" {
		expected = http.MethodGet
	}
	if r.Method != expected {
		w.Header().Set("Allow", expected)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodPost && !sameOrigin(r) {
		http.Error(w, "cross-origin mutation rejected", http.StatusForbidden)
		return
	}
	s := a.signedAnchorer()
	if s == nil {
		http.Error(w, "signed approvals are not configured (or legacy mode is enabled)", http.StatusServiceUnavailable)
		return
	}
	a.approvalMu.Lock()
	defer a.approvalMu.Unlock()
	d, ok := a.promos.Draft(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if action == "applied" {
		a.markApplied(w, r, d, s)
		return
	}
	doc, err := attest.ApprovalDocument(d)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if rec, ok := a.attests.For(id); ok && rec.Status == "anchored" {
		http.Error(w, "draft already anchored", http.StatusConflict)
		return
	}
	key, hash := attest.DraftKey(id), attest.DocHash(doc)
	cfg := s.SigningConfig()
	if err := cfg.Validate(); err != nil {
		http.Error(w, "invalid signing configuration", http.StatusServiceUnavailable)
		return
	}
	if action == "typed-data" {
		jsonResponse(w, http.StatusOK, struct {
			Document  string           `json:"document"`
			DocSHA256 string           `json:"doc_sha256"`
			TypedData attest.TypedData `json:"typed_data"`
			Chain     any              `json:"chain"`
		}{
			string(doc), attest.Hex32(hash), cfg.TypedData(key, hash),
			map[string]any{
				"chainId": fmt.Sprintf("0x%x", cfg.ChainID), "chainName": s.ChainName(),
				"nativeCurrency": map[string]any{"name": "Monad", "symbol": "MON", "decimals": 18},
				// Never expose the service's possibly credentialed RPC URL.
				"rpcUrls":           []string{envOr("SEKATS_WALLET_RPC", "https://rpc.monad.xyz")},
				"blockExplorerUrls": []string{cfg.Explorer},
			},
		})
		return
	}
	var input struct {
		Signature string `json:"signature"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields() // No client docHash, address, chain, or document.
	if err := dec.Decode(&input); err != nil {
		http.Error(w, "expected JSON containing only signature", http.StatusBadRequest)
		return
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		http.Error(w, "unexpected trailing JSON", http.StatusBadRequest)
		return
	}
	signer, err := cfg.VerifySignature(key, hash, input.Signature)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if d.Status == engine.DraftPending {
		if _, err := a.promos.Approve(id); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
	}
	rec := attest.Record{
		DraftID: id, DocSHA256: strings.TrimPrefix(attest.Hex32(hash), "0x"),
		Chain: s.ChainName(), Contract: s.Contract(), Attester: signer,
		Signature: input.Signature, DocSchema: "sekats-approval/v2", Status: "failed",
	}
	// Save the signed approval before the external side effect. Failure to relay
	// does not erase human consent; the same stable document may be retried.
	if err := a.attests.Put(rec); err != nil {
		http.Error(w, "could not persist approval; transaction not sent", http.StatusInternalServerError)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	rec.TxHash, err = s.AnchorWithSig(ctx, key, hash, input.Signature)
	status := http.StatusOK
	if err != nil {
		rec.Error = err.Error()
		status = http.StatusBadGateway
	} else {
		now := time.Now().UTC()
		rec.Status, rec.AnchoredAt = "anchored", &now
	}
	if err := a.attests.Put(rec); err != nil {
		http.Error(w, "relay completed but could not persist outcome; reconcile chain before retry", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, status, map[string]any{
		"status": rec.Status, "tx_hash": rec.TxHash, "tx_url": s.ExplorerTX(rec.TxHash),
		"attester": signer, "error": rec.Error,
	})
}

func (a *app) markApplied(w http.ResponseWriter, r *http.Request, d engine.Draft, s attest.SignedAnchorer) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(new(struct{})); err != nil {
		http.Error(w, "expected empty JSON object; client policy hashes and paths are not accepted", http.StatusBadRequest)
		return
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		http.Error(w, "unexpected trailing JSON", http.StatusBadRequest)
		return
	}
	rec, ok := a.attests.For(d.ID)
	if !ok || rec.Status != "anchored" || rec.DocSchema != "sekats-approval/v2" || d.Status != engine.DraftApproved {
		http.Error(w, "signed draft must be anchored before marking applied", http.StatusConflict)
		return
	}
	if !strings.EqualFold(rec.Contract, s.Contract()) || rec.Chain != s.ChainName() {
		http.Error(w, "approval belongs to a different contract or chain", http.StatusConflict)
		return
	}
	doc, err := attest.ApprovalDocument(d)
	if err != nil || rec.DocSHA256 != strings.TrimPrefix(attest.Hex32(attest.DocHash(doc)), "0x") {
		http.Error(w, "draft no longer matches its approved document", http.StatusConflict)
		return
	}
	if a.policyFile == "" {
		http.Error(w, "SEKATS_POLICY_FILE is not configured", http.StatusServiceUnavailable)
		return
	}
	f, err := os.Open(a.policyFile)
	if err != nil {
		http.Error(w, "cannot read configured policy file", http.StatusConflict)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 10<<20 {
		http.Error(w, "policy must be a regular file of at most 10 MiB", http.StatusConflict)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(f, (10<<20)+1))
	if err != nil || len(raw) > 10<<20 {
		http.Error(w, "cannot read policy file", http.StatusConflict)
		return
	}
	hash, err := attest.AppliedPolicyHash(raw, d)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	hashHex := strings.TrimPrefix(attest.Hex32(hash), "0x")
	if len(rec.Policies) > 0 {
		last := rec.Policies[len(rec.Policies)-1]
		if last.Status == "anchored" && last.PolicySHA256 == hashHex {
			jsonResponse(w, http.StatusOK, last)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	tx, operator, err := s.AnchorPolicy(ctx, attest.DraftKey(d.ID), hash)
	result := attest.PolicyRecord{
		PolicySHA256: hashHex, TxHash: tx, Attester: operator,
		RecordedAt: time.Now().UTC(), Status: "anchored",
	}
	status := http.StatusOK
	if err != nil {
		result.Status, result.Error = "failed", err.Error()
		status = http.StatusBadGateway
	}
	rec.Policies = append(rec.Policies, result)
	if err := a.attests.Put(rec); err != nil {
		http.Error(w, "could not persist policy outcome; reconcile chain before retry", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, status, result)
}
