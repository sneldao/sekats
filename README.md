# Sekats

**You can't `assertEquals` an agent.**

This is the human quality layer for a real autonomous poker agent — a DevFun
Arena bot that has made thousands of real-money decisions on Monad. Its own
advisory loop is broken: 6,243 shadow-LLM policy suggestions generated,
**6 promoted**. Its shadow-LLM review pipeline agrees with the agent's
actual decisions only **84.3%** of the time (1,497/1,775 linked reviews),
with a repeatable failure pattern (agent bets marginal spots, shadow-LLM
says check).

This app turns the agent's own strategy manual (`poker-policy.json`) into
checkable acceptance criteria, surfaces where its automated review pipeline
disagrees with itself, and gives a human a real queue to adjudicate those
disagreements.

Built in Go with server-rendered HTML + htmx — no JS build step, no
framework magic, on purpose. See `docs/PLAN.md` for the full rationale.

## Screens

1. **Quality Surface** (`/`) — the broken-loop callout, shadow-LLM
   agreement rate, review-provider error buckets.
2. **Policy-as-ACs** (`/policy`) — the policy file's hand-tier rules, run
   as assertions against real logged decisions. Currently catches 1 real
   violation in the sample.
3. **Review Queue** (`/review`) — 25 real hands where the agent and the
   shadow-LLM disagreed, with accept/reject buttons (htmx partial swap).
   Verdicts persist to disk and survive a restart.
4. **Trends** (`/trends`) — the shadow-LLM suggestion timeline (real
   timestamps from the fixture) and the disagreement pattern matrix
   (bet→check dominates). No fabricated agreement-over-time chart: the
   fixture carries no per-review timestamps, and the screen says so.
5. **Verdict Report** (`/verdicts`) — every human verdict joined against the
   fixture row it adjudicated, as a shareable markdown report; JSON export
   at `/export`.

## Data

All data in `data/` is a frozen snapshot exported from the live agent's
SQLite database (`arena-poker.sqlite3` on a remote host) — see
`docs/PLAN.md` for the export process. No live DB connection, no
credentials, in this repo.

## Run it

```
go build -o bin/server ./cmd/server
./bin/server
# http://localhost:8080
```

Override `ADDR`, `DATA_DIR`, `TEMPLATE_DIR`, `STATIC_DIR` via env vars if
needed.

## Kane CLI verification

See `docs/PLAN.md` for the closed-loop plan: Kane flows verify each of the
screens render their key numbers/callouts, that the review-queue
accept/reject action actually updates state and persists, and that the
verdict report/export reflect recorded verdicts.
