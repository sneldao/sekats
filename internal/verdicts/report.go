// Shareable verdict report: a markdown document joining every recorded
// human verdict with the fixture row it adjudicated, so a reviewer outside
// the console (or a future self) can see exactly what was decided and why,
// without running the app.

package verdicts

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sneldao/sekats/internal/store"
)

// JoinError flags one verdict whose decision_id had no matching
// disagreement fixture row — data drift, not a crash.
type JoinError struct {
	DecisionID int64
	Verdict    string
}

// BuildReport joins the recorded verdicts against the disagreement fixture
// rows. Verdicts that reference a decision ID absent from the fixtures are
// surfaced in Missing rather than silently dropped.
func BuildReport(queued []store.Disagreement, entries []Entry) (string, []JoinError) {
	byID := make(map[int64]store.Disagreement, len(queued))
	for _, d := range queued {
		byID[d.DecisionID] = d
	}

	accepted, rejected := 0, 0
	type row struct {
		e       Entry
		d       store.Disagreement
		missing bool
	}
	rows := make([]row, 0, len(entries))
	var missing []JoinError

	for _, e := range entries {
		d, ok := byID[e.DecisionID]
		switch {
		case ok && e.Verdict == "accepted":
			accepted++
		case ok && e.Verdict == "rejected":
			rejected++
		}
		if !ok {
			missing = append(missing, JoinError{DecisionID: e.DecisionID, Verdict: e.Verdict})
		}
		rows = append(rows, row{e: e, d: d, missing: !ok})
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].e.DecisionID < rows[j].e.DecisionID })

	var b strings.Builder
	b.WriteString("# Sekats — Human Verdict Report\n\n")
	fmt.Fprintf(&b, "Generated: %s\n\n", time.Now().UTC().Format(time.RFC1123))
	fmt.Fprintf(&b, "These are the calls a human made on the review queue: for each hand where the agent's action and the shadow-LLM review disagreed, which side did the human side with.\n\n")

	total := len(rows)
	fmt.Fprintf(&b, "| Agent (accepted) | Shadow-LLM (rejected) | Total |\n|---|---|---|\n| %d | %d | %d |\n\n", accepted, rejected, total)

	if total > 0 {
		b.WriteString("## Adjudications\n\n")
		for _, r := range rows {
			if r.missing {
				fmt.Fprintf(&b, "- decision_id %d: verdict %q, but no matching fixture row (data drift — export the fixtures again).\n", r.e.DecisionID, r.e.Verdict)
				continue
			}
			who := "the agent was right"
			if r.e.Verdict == "rejected" {
				who = "the shadow-LLM was right"
			}
			fmt.Fprintf(&b, "### %s on %s (%s) — %s\n\n", r.e.Verdict, r.d.HandCode, r.d.Street, who)
			fmt.Fprintf(&b, "- Agent action: `%s`  |  Shadow-LLM action: `%s`  |  Pot: %.2f\n", r.d.OurAction, r.d.LLMAction, r.d.Pot)
			fmt.Fprintf(&b, "- Recorded: %s\n\n", r.e.RecordedAt.Format("2006-01-02 15:04 UTC"))
			if rt := strings.TrimSpace(r.d.ResponseText); rt != "" {
				b.WriteString("Shadow-LLM response (trimmed):\n\n```\n")
				b.WriteString(rt)
				b.WriteString("\n```\n\n")
			}
		}
	}

	if len(missing) > 0 {
		b.WriteString("## Unmatched verdicts\n\n")
		for _, m := range missing {
			fmt.Fprintf(&b, "- decision_id %d (%s): not present in the frozen fixture export\n", m.DecisionID, m.Verdict)
		}
		b.WriteString("\n")
	}

	if total == 0 {
		b.WriteString("_No verdicts recorded yet. Adjudicate hands on the Review Queue screen; this report regenerates from the saved verdict file._\n")
	}

	return b.String(), missing
}
