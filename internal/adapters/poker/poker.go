// Package poker is the first sekats adapter: the DevFun
// Arena poker agent. It encodes the domain knowledge the engine stays
// agnostic to:
//
//   - the preflop hand-list sections of poker-policy.json that count as
//     checkable acceptance criteria;
//   - the pattern bucket for the agent's known rot (bet→check on marginal
//     postflop spots): the street plus the action pair;
//   - the shape of a drafted policy edit: a {pattern, rule} entry in
//     postflop.promotedFromHandReviews, the same section the agent's
//     existing six promotions live in, so an approved draft merges into
//     the live policy with no new machinery.
package poker

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sneldao/sekats/internal/engine"
	"github.com/sneldao/sekats/internal/live"
	"github.com/sneldao/sekats/internal/store"
)

const (
	// Name is the adapter registry key (SEKATS_ADAPTER=poker).
	Name = "poker"
	// Agent is the display name used in the UI.
	Agent = "DevFun Arena poker agent"
	// PolicyFileName is the data-dir file with the agent's strategy manual.
	PolicyFileName = "poker-policy.json"
	// RulesFileName is the data-dir file with this agent's declarative ACs.
	RulesFileName = "poker-rules.json"
	// PolicyTarget is where approved drafts land in the policy doc.
	PolicyTarget = "postflop.promotedFromHandReviews"
)

// Adapter is the poker adapter. It is stateless; one instance serves the
// whole process.
type Adapter struct{}

// New returns a poker adapter. Call adapters.Register(poker.New()) once at
// startup.
func New() *Adapter { return &Adapter{} }

func (a *Adapter) Name() string         { return Name }
func (a *Adapter) Agent() string        { return Agent }
func (a *Adapter) PolicyFile() string   { return PolicyFileName }
func (a *Adapter) RulesFile() string    { return RulesFileName }
func (a *Adapter) PolicyTarget() string { return PolicyTarget }

func (a *Adapter) LoadFixtures(dataDir string) (*store.Store, error) {
	return store.Load(dataDir)
}

func (a *Adapter) Labels() store.Labels {
	return store.Labels{Context: "Hand", Phase: "Street", Metric: "Pot"}
}

func (a *Adapter) DefaultLiveFile() string { return "live/arena-poker.sqlite3" }

func (a *Adapter) LoadLive(path string) (*store.Store, *live.SourceInfo, error) {
	return live.Load(path)
}

// preflopLists are the dot-path refs the declarative rules may address.
// Adding a new checkable section here is the whole change needed to make
// it usable from a rules file.
var preflopLists = map[string]string{
	"preflop.premium":                 "premium",
	"preflop.strong":                  "strong",
	"preflop.playableSuited":          "playableSuited",
	"preflop.playableOffsuitLate":     "playableOffsuitLate",
	"preflop.dominatedFoldCandidates": "dominatedFoldCandidates",
	"preflop.trashExamples":           "trashExamples",
}

type policyFile struct {
	Version int `json:"version"`
	Preflop struct {
		Premium                 []string `json:"premium"`
		Strong                  []string `json:"strong"`
		PlayableSuited          []string `json:"playableSuited"`
		PlayableOffsuitLate     []string `json:"playableOffsuitLate"`
		DominatedFoldCandidates []string `json:"dominatedFoldCandidates"`
		TrashExamples           []string `json:"trashExamples"`
	} `json:"preflop"`
}

// LoadPolicy parses the poker policy file and resolves the hand-list
// sections the declarative rules may reference.
func (a *Adapter) LoadPolicy(path string) (*engine.PolicyDoc, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc policyFile
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	sections := map[string][]string{
		"premium":                 doc.Preflop.Premium,
		"strong":                  doc.Preflop.Strong,
		"playableSuited":          doc.Preflop.PlayableSuited,
		"playableOffsuitLate":     doc.Preflop.PlayableOffsuitLate,
		"dominatedFoldCandidates": doc.Preflop.DominatedFoldCandidates,
		"trashExamples":           doc.Preflop.TrashExamples,
	}
	lists := make(map[string][]string, len(preflopLists))
	for ref, section := range preflopLists {
		lists[ref] = sections[section]
	}
	return engine.NewPolicyDoc(doc.Version, raw, lists), nil
}

// PatternKey buckets a disagreement so repeats accumulate: the spot class
// (street) plus the action pair. The agent's known rot — bets marginal
// turn/river spots, shadow-LLM says check — buckets exactly that way, and
// so does any other repeated (street, action→action) pattern.
func (a *Adapter) PatternKey(r engine.Review) string {
	phase := strings.TrimSpace(r.Phase)
	if phase == "" {
		phase = "unknown"
	}
	return strings.ToLower(phase) + ": " + r.OurAction + " to " + r.LLMAction
}

// NewDraft builds the staged policy edit for one bucket of rejections.
// ev is non-empty and sorted by decision ID ascending. The entry lands in
// postflop.promotedFromHandReviews as {pattern, rule} — the same shape the
// agent's existing promotions have.
func (a *Adapter) NewDraft(key string, ev []engine.Evidence) engine.Draft {
	first := ev[0]
	codes := make([]string, 0, len(ev))
	for _, e := range ev {
		if !containsString(codes, e.ContextCode) {
			codes = append(codes, e.ContextCode)
		}
		if len(codes) == 4 {
			break
		}
	}
	pattern := fmt.Sprintf("%s: agent %s, shadow-LLM %s — human rejected the agent %d times (%s)",
		first.Phase, first.OurAction, first.LLMAction, len(ev), strings.Join(codes, ", "))
	rule := fmt.Sprintf("Prefer %s over %s on this %s spot class; take the %s line only with a made hand or a clear drawing advantage.",
		first.LLMAction, first.OurAction, strings.ToLower(first.Phase), first.OurAction)

	return engine.Draft{
		ID:         Slug(key),
		PatternKey: key,
		Pattern:    pattern,
		Rule:       rule,
		Target:     PolicyTarget,
		Payload:    map[string]any{"pattern": pattern, "rule": rule},
		Evidence:   append([]engine.Evidence{}, ev...),
		Status:     engine.DraftPending,
		CreatedAt:  time.Now().UTC(),
	}
}

// Slug turns a pattern key into a stable draft ID ("flop: bet to check" →
// "flop-bet-to-check"), so one pattern key maps to one draft at a time.
func Slug(key string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(key)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case strings.ContainsRune("-_./ ", r):
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	return out
}

func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
