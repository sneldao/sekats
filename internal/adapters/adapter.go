// Package adapters is the pluggable seam between sekats' domain-neutral
// engine and a specific autonomous agent. An adapter knows:
//
//   - where the agent's policy file and declarative rules live, and which
//     policy sections resolve to checkable string lists;
//   - what a "repeated pattern" of rejected decisions looks like
//     (PatternKey buckets shadow-review disagreements so repeats
//     accumulate);
//   - what the auto-drafted policy edit should say and where it lands in
//     the policy doc (NewDraft / PolicyTarget).
//
// Adding a new agent means adding a new adapter (plus its policy file and
// rules JSON) — the engine, promotion pipeline, and UI stay untouched.
package adapters

import (
	"sort"
	"sync"

	"github.com/sneldao/sekats/internal/engine"
)

// Adapter bridges one domain to the engine.
type Adapter interface {
	// Name is the registry key (env SEKATS_ADAPTER selects among adapters).
	Name() string
	// Agent is the display name for the UI.
	Agent() string
	// PolicyFile / RulesFile are the data-dir file names of the agent's
	// policy and declarative acceptance criteria.
	PolicyFile() string
	RulesFile() string
	// LoadPolicy reads the policy file and resolves the string lists the
	// declarative rules may reference.
	LoadPolicy(path string) (*engine.PolicyDoc, error)
	// PatternKey buckets one shadow-review disagreement so that repeats of
	// the same spot class accumulate toward a policy draft.
	PatternKey(r engine.Review) string
	// NewDraft builds the staged policy edit for one bucket of human-
	// rejected reviews. ev is non-empty, sorted by decision ID ascending.
	NewDraft(key string, ev []engine.Evidence) engine.Draft
	// PolicyTarget is the dot path in the policy doc where approved drafts
	// land.
	PolicyTarget() string
}

var (
	mu       sync.RWMutex
	registry = map[string]Adapter{}
)

// Register makes an adapter available under its Name().
func Register(a Adapter) {
	mu.Lock()
	defer mu.Unlock()
	registry[a.Name()] = a
}

// Get returns the adapter registered under name.
func Get(name string) (Adapter, error) {
	mu.RLock()
	defer mu.RUnlock()
	a, ok := registry[name]
	if !ok {
		var known []string
		for n := range registry {
			known = append(known, n)
		}
		sort.Strings(known)
		return nil, &UnknownAdapterError{Name: name, Known: known}
	}
	return a, nil
}

// Names lists registered adapter names, sorted.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// UnknownAdapterError reports a request for an unregistered adapter.
type UnknownAdapterError struct {
	Name  string
	Known []string
}

func (e *UnknownAdapterError) Error() string {
	return "unknown adapter " + e.Name + " (registered: " + join(e.Known) + ")"
}

func join(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
