# Kane CLI Hackathon — Plan

**Date**: 2026-08-20
**Event**: TestMu AI Kane CLI Hackathon (19–21 Aug, submission deadline 21 Aug 11:59 PM IST / 11:29 AM PT)
**Lane**: Lane 2 (verification baked into workflow) / Lane 4 (requirements that test themselves) crossover
**Stack**: Go (`net/http` + `html/template`) + htmx, no JS build step
**Data source**: Real fixtures exported from the DevFun Arena poker agent (`../poker/`) via SQLite on `nuncio-vultr`, frozen as JSON — no live DB dependency for judges.

## TL;DR

The DevFun Arena poker agent (`../poker/`) is a real autonomous agent that has made thousands of real-money decisions on Monad, backed by a policy file, a shadow-LLM review pipeline, and a "promote good ideas into tests" loop. That loop is visibly broken: **6,243 policy suggestions generated, only 6 promoted** (0.1%), and the shadow-LLM review pipeline has an **84.3% agreement rate** (1,497/1,775 linked reviews) with a repeatable pattern of disagreement (agent bets, shadow-LLM says check, on marginal turn/river spots).

This app is the human quality layer for that broken loop: it turns the agent's own strategy manual (`poker-policy.json`) into checkable assertions, surfaces where the automated review pipeline disagrees with the agent, and gives a human a queue to accept/reject disagreements — the artifact "you can't `assertEquals` an agent" argues you need.

## Why Go + htmx (the risk we're taking)

- The field is a JS-framework flood (Next.js/Astro/Remix dashboards). A server-rendered, no-build-step Go app is a genuine craft differentiator and pairs narratively with the theme: a deterministic, typed, no-runtime-magic language building the verification console for a policy-driven agent.
- Kane CLI drives a real browser against rendered DOM — it does not care about the framework, only that flows are stable and real. Server-rendered HTML + htmx partial swaps is Kane-friendly (few dynamic client states to describe).
- Risk accepted: less muscle-memory than React. Mitigated by keeping scope to 3 screens and using `html/template` + htmx (not a heavier Go web framework) to minimize unknowns under the deadline.

## Screens (all server-rendered, htmx for partial updates)

1. **Quality Surface** (`/`) — bb/100 trend (from `.arena-poker-state` history + tournament rank), shadow-LLM agreement rate, provider health (Featherless/NVIDIA/OpenRouter/Venice error buckets from `llm_reviews.error`), the "broken loop" callout (suggestions vs promotions).
2. **Policy-as-ACs** (`/policy`) — load `poker-policy.json` rules, run them as assertions against the decision fixtures (e.g. "dominatedFoldCandidates hand raised" → violation), list pass/fail per rule category.
3. **Review Queue** (`/review`) — list of real decisions where the agent and shadow-LLM disagreed (from `disagreements_sample` fixture), htmx-driven accept/reject buttons that record a human verdict, persisted to `data/verdicts.json`.
4. **Trends** (`/trends`) — shadow-LLM suggestion timeline (real epoch-ms `created_at` values from the fixture) and the disagreement pattern matrix behind the 84.3% headline.
5. **Verdict Report** (`/verdicts`) — every recorded human verdict joined against the fixture row it adjudicated, rendered as a shareable markdown report; raw JSON at `/export`.

## Data fixtures (frozen, real — `data/`)

- `decisions_export.json` — recent real decisions (`tools/export_scenarios.py` output).
- `quality_stats.json` — policy-suggestion counts, llm_reviews error buckets, linked agreement rate, disagreement sample (25 real disagreements with hand code, street, pot, shadow-LLM rationale).
- `poker-policy.json` — copy of the live policy file, source of truth for the AC engine.

## Kane CLI closed loop (for the video)

- Kane flow 1: verify the Quality Surface renders the broken-loop callout and agreement rate.
- Kane flow 2: verify a Policy-as-ACs violation is flagged (seed one deliberately-violating fixture row if none exist naturally).
- Kane flow 3: verify the Review Queue accept/reject action updates state and Kane can observe the change.
- Kane flow 4: verify a queue verdict appears in the Verdict Report and JSON export, and survives a server restart.
- Hook: a `verify_with_kane` step runs on save; if a flow fails, the agent reads Kane's NDJSON output, fixes the template/handler, re-runs. Capture one real instance of this for the demo.

## Repo hygiene

- Repo initialized 2026-08-20 (within the "on/after 19 Aug" eligibility window; checked via `git log`).
- The poker repo (`../poker/`) is the data source only — untouched, not part of the submission repo.
- No credentials committed. Fixtures are exported snapshots, not live DB connections.

## Build order (1-day scope)

1. Go module skeleton + `html/template` layout + htmx script tag. ✅ (this commit)
2. `internal/store`: load fixture JSON into typed structs.
3. `internal/quality`: compute/format the quality-surface numbers from `quality_stats.json`.
4. `internal/policy`: parse `poker-policy.json`, define assertion rules, run against `decisions_export.json`.
5. `internal/review`: in-memory review-queue state + accept/reject handlers.
6. Templates + minimal CSS for all 3 screens.
7. Kane flows (`kane/*.flow` or equivalent) for the 3 screens + one closed-loop demo. ✅
8. README with setup + 3-min demo script. ✅

## Progress log — build-week extension (2026-08-26)

Scope added during the AgnesCode Build Challenge build week (original Kane CLI
hackathon scope above was complete as of Aug 21; this is the second build pass):

1. **Verdict persistence** (`internal/verdicts`) — review-queue accept/reject
   verdicts were in-memory only; they now persist to `data/verdicts.json`,
   flush on every write, and reload on startup. Covered by unit tests
   (`verdicts_test.go`: round-trip across reload, missing-file tolerance,
   overwrite semantics).
2. **Verdict Report** (`/verdicts`, `internal/verdicts/report.go`) — a
   shareable markdown report joining every recorded human verdict to the exact
   fixture row it adjudicated. Verdicts referencing decision IDs absent from
   the frozen fixture are surfaced as "unmatched" (data drift) instead of
   silently dropped — same honesty rule as the rest of the app.
3. **JSON export** (`/export`) — full queue state with counts and per-item
   verdicts as downloadable JSON (`?download=true`) plus an in-page preview.
4. **Trends screen** (`/trends`, `internal/trends`) — the suggestion timeline
   (real epoch-ms `created_at` values from `decisions_export.json`; statuses:
   pending/error) and the disagreement pattern matrix behind the 84.3%
   headline. Deliberately *no* agreement-rate-over-time chart: the fixture
   carries no per-review timestamps, so that series would be fabricated. The
   screen says so explicitly.
5. **Kane flow 4** (`kane/verdict_report_test.md`) — end-to-end: accept a hand,
   see it in the Verdict Report and JSON export, restart the server, confirm
   persistence.
6. **Honesty fixes** — README's disagreement count corrected 26 → 25 (the
   fixture holds 25); Trends subtitle no longer claims a preflop analysis it
   doesn't contain.

State at end of build week: all 5 screens live, `go test ./...` green,
`go vet ./...` clean, 4 Kane flows authored.

Diagnostics proof of Agnes model usage (organizer-approved in lieu of an
AgnesCode-app export): `docs/agnes-diagnostics/` — two raw session transcripts
(pi CLI on `agnes-2.5-flash`, Factory droid on `Agnes-Code-2.5-Pro-0`, both
2026-08-26 against this repo) plus a summary and verification commands.

## Progress log — live bridge + closed promotion loop (2026-08-26)

Three features added on top of the build-week state:

### 1. Read-only live bridge (`internal/live`, `tools/sync_live.sh`)

The frozen fixtures in `data/` were a 2026-08-20 export; the console could
never show fresh rot. Now:

- `tools/sync_live.sh` snapshots the live DB (`nuncio-vultr`,
  `/home/linuxuser/arena-poker/arena-poker.sqlite3`, WAL mode, ~142 MB)
  with SQLite's online-backup API over SSH — a WAL-consistent copy, not a
  racing rsync of `.sqlite3`/`-wal`/`-shm` — into `data/live/`, with an
  atomic rename so a half-written file is never the visible snapshot.
- The server auto-detects `data/live/arena-poker.sqlite3` (or
  `SEKATS_DB=/path`, `SEKATS_DB=off` to force fixtures), opens it
  read-only (URI `mode=ro` + `PRAGMA query_only`), and recomputes every
  quality number with the exact semantics the fixture export used (linked
  = `decision_id IS NOT NULL`; agreement over linked error-free reviews;
  error buckets over all reviews; disagreement sample = latest 25 linked
  error-free mismatches). Verified against the live snapshot: the bridge
  reproduces the fixture's 6243/6, 84.3%, 2712, and the same 25-hand
  sample.
- Freshness is per-request: the snapshot's mtime is re-checked on each
  page load, so a new sync is reflected without a restart; load failures
  (e.g. a mid-copy file) keep the previous data.
- Honesty in the UI: the Quality Surface carries a source badge
  (LIVE with sync time + latest decision id/time, or FROZEN with the
  export date and a pointer at the sync tool), and the broken-loop
  callout states whether its numbers are current or stale.
- Security boundary: no credentials or host access in this repo (public
  canary); the sync script uses the operator's own SSH alias, and the
  142 MB snapshot + all local state files are git-ignored.

### 2. Closed promotion loop (`internal/promotion`, `/promotions`)

The 0.1% promotion rate became a working human-in-the-loop pipeline:

- Rejecting a hand in the review queue buckets it by pattern (adapter's
  `PatternKey`: street + action pair, e.g. `flop: bet to check`) with the
  decision id, hand, pot, and the shadow-LLM's extracted rationale as
  evidence; state persists in `data/pattern_rejections.json` and dedupes
  by decision id (one hand counts once).
- A pattern crossing the threshold (`SEKATS_PROMOTE_THRESHOLD`, default 3)
  auto-drafts a policy edit: a `{pattern, rule}` entry for
  `postflop.promotedFromHandReviews` — the same section the agent's
  existing six promotions live in, so an approved edit merges into the
  live policy with no new machinery. The toast appears in the review
  queue and the draft is staged on `/promotions` with its evidence table.
  A discarded pattern re-enters review if it keeps recurring; approved is
  terminal. State persists in `data/policy_drafts.json`.
- Approval produces a machine-applicable `sekats-policy-patch/v1` JSON
  (target, entry, source decisions, provenance) downloadable from
  `/promotions/{id}/patch`; `tools/apply_policy_draft.py` is the only
  writer — it runs on whatever machine holds the live policy file, is
  idempotent, and refuses unknown schemas. The console never writes to
  agent files (public/private host split from the poker repo's AGENTS.md).
- Verified end-to-end in a real browser (Kane flow `promotions_test.md`):
  reject three flop bet→check hands → auto-draft with 3 evidence rows →
  approve → patch JSON with correct schema/target/entry.

### 3. Pluggable schema (`internal/engine`, `internal/adapters`)

Poker-specific rules moved out of Go code into data:

- `internal/engine` is domain-neutral: generic `Decision`/`Review`,
  declarative `Rule` (phase + actions + policy list ref + detail
  template) loaded from per-agent rules JSON, `RunRules`, `Draft`/`Patch`,
  rationale extraction.
- `internal/adapters` is the seam: the poker adapter resolves the
  preflop hand-list sections of `poker-policy.json`, owns `PatternKey`
  and the draft text/target, and registers under `SEKATS_ADAPTER`
  (default `poker`). The three AC rules now live in
  `data/poker-rules.json` — the old hard-coded functions are gone.
- The promotion pipeline, UI, and persistence are adapter-agnostic; a new
  agent with a policy file and decision log adds one adapter plus its
  policy and rules files. The remaining poker-specific surface is the
  decision-log SQL in `internal/live` (documented as the next seam).

State at end of pass: `go test ./...` green (engine, adapters, live
bridge against a real temp SQLite DB, promotion, policy, verdicts),
`go vet` clean, server verified in both fixture and live modes, three
Kane flows added/updated (`live_bridge_test.md`, `promotions_test.md`,
and `review_queue_test.md` now targets a pending item specifically so the
suite stays re-runnable against persisted state).

## Progress log — on-chain attestation, Monad (2026-10-13)

Added for the Monad Metropolis hackathon window: human approvals now
anchor on Monad mainnet (chain 143), closing the audit trail on the same
chain where the agent risks real MON.

- `internal/attest` — canonical `sekats-attestation/v1` doc (byte-stable:
  fixed-at-approval fields only, sorted map keys), SHA-256 digest, record
  store (`data/attestations.json`, git-ignored), and a `cast send`
  anchorer selected by env (`SEKATS_ATTEST_KEY` + `SEKATS_ATTEST_CONTRACT`
  → enabled; `SEKATS_ATTEST=off` or missing config → honestly disabled).
  Key material lives only in the operator's env — never in the repo, the
  data files, or on the public parlor.
- `contracts/SekatsAttestations.sol` — append-only
  `anchor(bytes32 draftKey, bytes32 docHash)` registry
  (`draftKey = sha256("sekats/draft/" + draftID)`); one anchor per draft,
  re-anchoring reverts. Committed creation bytecode
  (`contracts/SekatsAttestations.bin`, solc 0.8.26 --optimize) so deploy
  needs only `cast`: `tools/deploy_attestations.sh`.
- Routes: `GET /promotions/{id}/attestation` serves the doc with an
  `X-Sekats-Doc-SHA256` header for offline verification;
  `POST /promotions/{id}/attest` retries a failed anchor.
- UI: anchored/failed/off state on each approved draft card (tx link to
  monadscan, truncated doc hash), an "Anchored on Monad" tile in the
  pipeline counts, and an anchored counter on the Quality Surface.

Verified locally in fixture mode with no signer: draft auto-stages on
3 rejections, approve renders the honest "not configured" state, and the
attestation doc is byte-identical across requests (header digest ==
`shasum -a 256`). On-chain end-to-end pending deploy of the contract.
