# TGGLEWARS — Flagsmith Experimentation Hackathon entry

**Date:** 2026-11-25 (implementation status updated 2026-11-26)
**Event:** TGGLEWARS, WeMakeDevs × Flagsmith — Nov 26–29, 2026 (hacking 8 AM IST Nov 26 → submissions 8 PM IST Nov 29)
**Track:** 02 — AI Agents and Feature Flags ($1,000 cash)
**Entry type:** extension of this existing repo. The hackathon-specific integration is consolidated into a single commit published inside the official hacking window; the rules permit head starts/existing projects, but confirm in the Flagsmith Discord anyway.

## TL;DR

Sekats exists because the poker agent's self-improvement loop is broken: **6,243 shadow-LLM policy suggestions generated, 6 promoted**, and the shadow review pipeline agrees with actual decisions only **84.3%** of the time. Today a promotion is binary — a human approves a policy draft and the patch is meant to go live, full stop. That binary gate is exactly what feature flags exist to replace.

**The pitch:** wire Flagsmith in as the rollout layer between "human approves a policy draft" and "the policy patch earns full application." An approved draft is measured behind a multivariate flag at partial rollout, not treated as proven wholesale — and Flagsmith Experimentation measures whether it actually helped before it earns full rollout. Flags stop being decoration: they are the mechanism that turns a broken binary loop into a graduated one.

## Implementation status — 2026-11-26

Implemented in the consolidated hackathon integration:

- `internal/flags` REST client for the Management API, Edge identity evaluation, Events API ingestion, and results reads.
- Human approval (local or wallet-signed) provisions the managed warehouse, flag, keyed candidate variant, metrics, and experiment, then starts the experiment.
- Deterministic `decision-<id>` replay over human-labelled queue rows. Pending rows are excluded; off-pattern rows keep control behaviour in both arms; remote evaluation failures are explicit control fallback.
- `$flag_exposure`, mean human-verdict alignment, and summed policy-violation events; durable local rollout evidence in `flagsmith_rollouts.json`; promotion-card and home-surface rendering.
- Retry behaviour that accounts for asynchronous edge/warehouse propagation: zero-exposure retries replay again; once exposures are accepted, retry refreshes results instead of duplicating conversions.
- `go vet ./...` and `go test ./...` pass, including a mocked end-to-end provisioning/replay test.

**Live Cloud validation — 2026-11-26:** the approved `flop-bet-to-check` fixture provisioned successfully in Flagsmith Cloud: managed warehouse connection, multivariate flag `sekats_policy_flop_bet_to_check`, keyed candidate variant, two metrics, and running experiment `sekats-flop-bet-to-check` (id 638). Edge evaluation returned experiment enrolment for deterministic decision identities; the canonical client key correctly did not reveal the server-key-only flag while the `ser.` key did. The Events API accepted three exposures plus three alignment events (six total warehouse events), and replay produced control 0/1 aligned versus candidate 2/2 aligned with zero violations. Warehouse results are intentionally still pending/inconclusive because the fixture is far below the 50-identities-per-variant inference floor; no significance is claimed. The public console deployment is still the pre-Flagsmith build. Required credentials are documented in `README.md`; use the git-ignored `data/state/flagsmith.env`, never commit them.

**Deliberately not implemented:** a win does not automatically roll the candidate to 100%, complete the experiment, retire the draft, or apply the patch. This slice stops at evidence; the operator downloads/applies the patch externally. Automating experiment completion or full rollout is a stretch goal, not part of the committed implementation.

## Why this is a strong Track 02 entry

Judging criteria for Track 02, mapped:

- **Flagsmith CLI, API or MCP server** — the entry uses the Management API, Edge evaluation API, Events API, and experiment results API. The MCP server is intentionally not the lifecycle path because it cannot start experiments.
- **Saves real time** — today every one of the 6,237 unpromoted suggestions is dead work; a graduated rollout path makes more of the pipeline's output usable, because the cost of a wrong "accept" drops.
- **A human approves changes** — already sekats' core philosophy. Flagsmith adds what comes *after* approval: staged rollout and measurement. Human stays on the gate; flags own the gradient.
- **Clear output** — the quality surface renders the loop's numbers, rollout state, local replay counts, dashboard links, and either win probability or an honest collecting-data state.

The demo narrative writes itself: real money, real agent, real broken loop with real numbers — fixed with flags. Very few entries will have a pre-existing, quantified problem statement.

## The design

### What gets flagged

Policy drafts in the promotion pipeline. Today: repeated human rejections stage a draft, then a human approves or discards it. After:

```
repeated rejections → auto-draft → human approves → Flagsmith experiment at partial rollout →
deterministic replay sends exposure/conversion events → evidence informs the operator's patch decision
```

- Each approved draft gets a **multivariate flag** in Flagsmith: `control` (current policy) vs keyed `candidate` variant (suggested policy).
- The rollout percentage starts small (default 20%). Because the harness replays decisions from fixtures, "traffic split" is deterministic per decision id — no RNG wobble, replays stay reproducible.
- **Metrics:** mean human-verdict alignment (a numeric 1/0 event against the action a human adjudicated as correct) and summed policy-violation count from the policy-as-AC engine as the guardrail. Pending unlabelled rows are excluded rather than counted as failures. Event timestamps are replay time, not original decision time, so the experiment measures a deterministic retrospective cohort rather than live traffic chronology. Human-verdict alignment is a retrospective validation signal, not proof of profitability.

### Integration surface

- **Flagsmith Management API + Edge evaluation + Events API** — sekats itself provisions the warehouse, flag, metrics and experiment; evaluates deterministic decision identities; and writes exposure/conversion events.
- **Human approval stays in the promotion UI** — the flag is created *when* a human approves a draft, never before. This is the answer to "what the agent does on its own and where it stops to ask a person": the agent proposes and measures, the human commits.

### Experiment endgame

Flagsmith Experimentation's Bayesian engine reports win probability per variant once its data floor is met (currently at least 50 exposed identities per variant, with further statistical edge cases). The committed UI reports that probability when available and otherwise says data is still being collected. It does **not** fabricate extra traffic or pretend a tiny fixture is significant.

The operational endgame remains human-controlled: inspect local replay rows and warehouse evidence, then apply or retire the patch externally. A future extension can use the experiment completion/rollout API to promote a winning variant or disable a losing one, but that automation is currently out of scope.

## Scope for a 4-day build

Realistic slice, in order:

1. ✅ Use Flagsmith Cloud free tier; create project + environment. Docker is unavailable locally, so Cloud is the chosen path.
2. ✅ `internal/flags` REST client: remote deterministic identity evaluation, explicit control fallback when Flagsmith is unreachable — the quality layer must never take the agent down.
3. ✅ Promotion pipeline: approve → create flag + initial rollout via Flagsmith API, surfaced on the draft card.
4. ✅ Experiment wiring: replayed decision outcomes tagged by variant, fed as events; quality surface shows experiment state and win probability when Flagsmith has enough data.
5. ✅ Validate the flow against live Flagsmith Cloud, including edge propagation, event acceptance, warehouse refresh, and the honest data-floor state.
6. ☐ Demo video. The README setup/usage section is done.

## Submission checklist (rules)

- [x] Public GitHub repo with README: setup steps + how Flagsmith is used
- [ ] Short demo video: the broken-loop numbers → approve a draft → flag created → partial rollout → replay/exposure events → warehouse collecting-data state or verdict → human patch decision
- [ ] Register at wemakedevs.org/hackathons/flagsmith before the deadline (every team member)
- [ ] Submit before 8 PM IST, Nov 29 — no late submissions

## Constraints carried from this repo

- `../poker/` is the data source only — untouched, not part of the submission. Flags gate the *evaluation and promotion harness*, not the live agent's wallet path; nothing in this hackathon touches real money decisions directly.
- No credentials committed; Flagsmith API keys via env or the git-ignored `data/state/flagsmith.env` loaded into the process at runtime.
- Same honesty rule as the rest of the app: losing variants are reported as losses, inconclusive experiments stay inconclusive, and an experiment that never shows a loss is a demo, not an experiment.
