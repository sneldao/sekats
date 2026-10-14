package main

import (
	"encoding/json"
	"net/http"
)

// handleJury serves the juror roster reconstructed from SekatsJury's
// JurorVerdict logs, enriched with Nansen labels when configured.
func (a *app) handleJury(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/jury" {
		http.NotFound(w, r)
		return
	}
	if !readOnlyHandler(w, r) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(a.juryWatcher.Snapshot(r.Context()))
}

// handleWalletLabels serves Nansen Profiler labels for one address.
// Config-free installs answer with an empty, honestly-labelled result.
func (a *app) handleWalletLabels(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/wallet-labels" {
		http.NotFound(w, r)
		return
	}
	if !readOnlyHandler(w, r) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(a.enricher.Lookup(r.Context(), r.URL.Query().Get("address")))
}
