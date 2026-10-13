package attest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sneldao/sekats/internal/engine"
)

// AppliedPolicyHash requires the approved entry at its target before hashing
// the exact file bytes, including whitespace. It never trusts a supplied digest.
func AppliedPolicyHash(raw []byte, d engine.Draft) ([32]byte, error) {
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return [32]byte{}, fmt.Errorf("policy file is not valid JSON")
	}
	node := root
	for _, part := range strings.Split(d.Target, ".") {
		m, ok := node.(map[string]any)
		if !ok || part == "" {
			return [32]byte{}, fmt.Errorf("policy target is missing")
		}
		node, ok = m[part]
		if !ok {
			return [32]byte{}, fmt.Errorf("policy target is missing")
		}
	}
	entries, ok := node.([]any)
	if !ok {
		return [32]byte{}, fmt.Errorf("policy target is not an array")
	}
	want, err := json.Marshal(d.Payload)
	if err != nil {
		return [32]byte{}, fmt.Errorf("cannot encode approved entry")
	}
	// Normalize JSON number representation on both sides.
	var normalized any
	if err := json.Unmarshal(want, &normalized); err != nil {
		return [32]byte{}, err
	}
	want, _ = json.Marshal(normalized)
	for _, entry := range entries {
		got, err := json.Marshal(entry)
		if err == nil && bytes.Equal(got, want) {
			return DocHash(raw), nil
		}
	}
	return [32]byte{}, fmt.Errorf("approved patch entry is not present in resulting policy")
}
