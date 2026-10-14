package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sneldao/sekats/internal/attest"
	"github.com/sneldao/sekats/internal/enrich"
	"github.com/sneldao/sekats/internal/monad"
)

type anchorRow struct {
	DraftID    string         `json:"draftID"`
	DraftKey   string         `json:"draftKey"`
	DocHash    string         `json:"docHash"`
	Attester   string         `json:"attester"`
	AttesterLabels []enrich.Label `json:"attesterLabels,omitempty"`
	TxHash     string         `json:"txHash"`
	TxURL      string     `json:"monadscanURL"`
	Contract   string     `json:"contract"`
	DocURL     string     `json:"docURL"`
	CastCall   string     `json:"castCall"`
	AnchoredAt *time.Time `json:"anchoredAt,omitempty"`
}

type anchorRegistry struct {
	RPC      string      `json:"rpc"`
	Contract string      `json:"contract"`
	Anchors  []anchorRow `json:"anchors"`
}

func (a *app) anchorRegistry(ctx context.Context) anchorRegistry {
	view := anchorRegistry{RPC: a.publicRPC, Contract: a.attestContract, Anchors: make([]anchorRow, 0)}
	for _, rec := range a.attests.Anchored() {
		contract := rec.Contract
		if contract == "" {
			contract = a.attestContract
		}
		row := anchorRow{
			DraftID: rec.DraftID, DraftKey: fmt.Sprintf("0x%x", attest.DraftKey(rec.DraftID)),
			DocHash:  "0x" + strings.TrimPrefix(rec.DocSHA256, "0x"),
			Attester: rec.Attester, TxHash: rec.TxHash, Contract: contract,
			DocURL:     "/promotions/" + url.PathEscape(rec.DraftID) + "/attestation",
			AnchoredAt: rec.AnchoredAt,
		}
		if rec.TxHash != "" {
			row.TxURL = strings.TrimRight(a.attestExplorer, "/") + "/tx/" + url.PathEscape(rec.TxHash)
		}
		if rec.Attester != "" {
			if res := a.enricher.Lookup(ctx, rec.Attester); len(res.Labels) > 0 {
				row.AttesterLabels = res.Labels
			}
		}
		if monad.ValidAddress(contract) && monad.ValidRPC(view.RPC) {
			row.CastCall = "cast call " + contract + ` "anchored(bytes32)(bytes32)" ` + row.DraftKey +
				" --rpc-url " + shellQuote(view.RPC)
		}
		view.Anchors = append(view.Anchors, row)
	}
	return view
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func readOnlyHandler(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	w.Header().Set("Allow", "GET, HEAD")
	http.Error(w, "read-only endpoint", http.StatusMethodNotAllowed)
	return false
}

func (a *app) handleAnchors(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/anchors" {
		http.NotFound(w, r)
		return
	}
	if !readOnlyHandler(w, r) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	a.renderPage(w, "page-anchors", "Anchors Registry", a.anchorRegistry(r.Context()))
}

func (a *app) handleAnchorsJSON(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/anchors" {
		http.NotFound(w, r)
		return
	}
	if !readOnlyHandler(w, r) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(a.anchorRegistry(r.Context()))
}
