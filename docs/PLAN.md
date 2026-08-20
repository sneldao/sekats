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
3. **Review Queue** (`/review`) — list of real decisions where the agent and shadow-LLM disagreed (from `disagreements_sample` fixture), htmx-driven accept/reject buttons that record a human verdict (in-memory or flat file — no external DB needed for the demo).

## Data fixtures (frozen, real — `data/`)

- `decisions_export.json` — recent real decisions (`tools/export_scenarios.py` output).
- `quality_stats.json` — policy-suggestion counts, llm_reviews error buckets, linked agreement rate, disagreement sample (26 real disagreements with hand code, street, pot, shadow-LLM rationale).
- `poker-policy.json` — copy of the live policy file, source of truth for the AC engine.

## Kane CLI closed loop (for the video)

- Kane flow 1: verify the Quality Surface renders the broken-loop callout and agreement rate.
- Kane flow 2: verify a Policy-as-ACs violation is flagged (seed one deliberately-violating fixture row if none exist naturally).
- Kane flow 3: verify the Review Queue accept/reject action updates state and Kane can observe the change.
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
7. Kane flows (`kane/*.flow` or equivalent) for the 3 screens + one closed-loop demo.
8. README with setup + 3-min demo script.
