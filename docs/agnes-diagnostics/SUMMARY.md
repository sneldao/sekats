# Agnes Model Usage — Diagnostics Bundle

**Project**: sekats (AgnesCode Build Challenge, build week 20–26 Aug 2026)
**Purpose**: proof of Agnes model usage during the build week, per submission
requirements. The organizers confirmed on Discord (#hackathon-chat) that raw
agent-CLI session transcripts are acceptable diagnostics evidence in place of
an AgnesCode-app export.

## Sessions included

| File | Tool | Model | Session window (UTC) | Events |
|---|---|---|---|---|
| `pi-cli_agnes-2.5-flash_sekats_2026-08-26.jsonl` | pi CLI | `agnes-2.5-flash` | 2026-08-26 12:58:42 → 13:24:39 | 262 (71 model-tagged turns) |
| `factory-droid_agnes-code-2.5-pro_sekats_2026-08-26.jsonl` | Factory droid CLI | `custom:Agnes-Code-2.5-Pro-0` | 2026-08-26 12:52:07 → 13:06:17 | 46 (19 assistant turns) |

Both sessions ran with working directory `/Users/<user>/Dev/sekats` — the
submitted repository. Both fall inside the build-week window.

Note: the transcripts reference the Go module path
`github.com/udingethe/sekats`, which was renamed to `github.com/sneldao/sekats`
immediately after these sessions (the account holder does not control the
`udingethe` GitHub org). The submitted code and `go.mod` use the `sneldao`
path; the transcripts are preserved as-is as historical records.

## What the models did

The two sessions implemented the build-week extension documented in
`docs/PLAN.md` ("Progress log — build-week extension"):

- `internal/verdicts/verdicts.go`, `report.go` — verdict persistence and the
  shareable Verdict Report (`/verdicts`)
- `internal/trends/trends.go` + `templates/page-trends.html` — the Trends
  screen (`/trends`)
- `/export` JSON endpoint in `cmd/server/main.go`; nav wiring in
  `templates/layout.html`
- `kane/verdict_report_test.md` — Kane flow 4
- `internal/verdicts/verdicts_test.go` — unit tests for persistence and report
  joins
- README / docs honesty fixes (disagreement count corrected 26 → 25)

## How to verify

```
# count model calls in each transcript
grep -c '"model":"agnes-2.5-flash"' pi-cli_agnes-2.5-flash_sekats_2026-08-26.jsonl
grep -o '"modelId":"[^"]*"' factory-droid_agnes-code-2.5-pro_sekats_2026-08-26.jsonl | sort | uniq -c

# confirm timestamps fall inside the build week
python3 -c "
import json
for f in ['pi-cli_agnes-2.5-flash_sekats_2026-08-26.jsonl',
          'factory-droid_agnes-code-2.5-pro_sekats_2026-08-26.jsonl']:
    ts=[json.loads(l).get('timestamp') for l in open(f) if l.strip() and json.loads(l).get('timestamp')]
    print(f, ts[0], '->', ts[-1])
"
```

Originals of both transcripts live outside this repo at
`~/.pi/agent/sessions/--Users-udingethe-Dev-sekats--/` and
`~/.factory/sessions/-Users-udingethe-Dev-sekats/`.
