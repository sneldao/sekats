# Sekats

> **TGGLEWARS (Nov 26–29, 2026):** this repo is the Track 02 entry for the
> WeMakeDevs × Flagsmith hackathon — plan and implementation status in
> [`docs/TGGLEWARS.md`](docs/TGGLEWARS.md).

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
disagrees with itself, gives a human a real queue to adjudicate those
disagreements, and — closing the loop that produced the 0.1% promotion
rate — auto-drafts a policy edit when a human keeps rejecting the agent on
a repeated pattern, stages it for review, and produces an applyable patch
on approval.

Built in Go with server-rendered HTML + htmx — no JS build step, no
framework magic, on purpose. See `docs/PLAN.md` for the full rationale.

**Live demo:** <https://console.sekats.thisyearnofear.com> — fixture mode,
no credentials on the host. Bring a wallet to leave a juror verdict on
`/review` or to verify an anchor client-side on `/anchors`. At the time of
writing this deployment lags `master` and does not yet demonstrate the
Flagsmith integration; run the credentialed local flow below for that work.

**The parlor:** <https://sekats.thisyearnofear.com> — the public canary
that predates this console. It answers one question: does the agent's
claimed contract still match the live arena (HOLD/DRIFT)? The console is
the governance surface; the parlor is the proof-of-honesty artifact. The
two link to each other.

## Screens

1. **Quality Surface** (`/`) — the broken-loop callout, shadow-LLM
   agreement rate, review-provider error buckets, active Flagsmith rollout state,
   a data-source badge
   stating whether the numbers are LIVE (read-only bridge) or FROZEN
   (stale export), and a read-only treasury line reading the agent's
   Monad balance and transaction count live from RPC.
2. **Policy-as-ACs** (`/policy`) — the policy file's hand-tier rules, run
   as assertions against real logged decisions. The rules are declarative
   JSON (`data/poker-rules.json`), not Go code. Currently catches 1 real
   violation in the sample.
3. **Review Queue** (`/review`) — 25 real hands where the agent and the
   shadow-LLM disagreed, with accept/reject buttons (htmx partial swap).
   Verdicts persist to disk and survive a restart. Rejecting a hand also
   feeds the pattern buckets below. When `SEKATS_JURY_CONTRACT` is set,
   each row also carries permissionless **juror verdicts** — any visitor's
   wallet can side with the agent or the shadow on Monad, and tallies are
   read back from the chain.
4. **Promotion Pipeline** (`/promotions`) — the closed loop: rejections on
   a repeated pattern (default: 3 per pattern) auto-draft a policy edit
   into `postflop.promotedFromHandReviews` (the same section the agent's
   existing promotions live in), with the rejected hands as evidence. A
   human approves (→ downloadable `.policy-patch.json`) or discards.
   Nothing is ever applied automatically. Approval also provisions a
   graduated Flagsmith experiment: control vs candidate at a small rollout,
   with replay evidence and warehouse significance rendered on the approved
   draft. Approvals can additionally be
   **signed with a human wallet and anchored on Monad**: an owner-managed
   approver signs the canonical document hash with EIP-712. The contract
   records that signer, not the service relayer. Resulting policy-file
   hashes can then be anchored as append-only lineage.
5. **Trends** (`/trends`) — the shadow-LLM suggestion timeline (real
   timestamps from the fixture) and the disagreement pattern matrix
   (bet→check dominates). No fabricated agreement-over-time chart: the
   fixture carries no per-review timestamps, and the screen says so.
6. **Verdict Report** (`/verdicts`) — every human verdict joined against the
   fixture row it adjudicated, as a shareable markdown report; JSON export
   at `/export`.
7. **Anchors Registry** (`/anchors`, JSON at `/api/anchors`) — every successful
   record from the persisted attestation store, including drafts no longer in
   the queue, with contract, draft key, document hash, attester, transaction
   link and a copyable read-only `cast call`. The browser independently hashes
   the canonical document and reads the contract getter.

## Data: frozen fixtures and the live bridge

All data in `data/` is a frozen snapshot exported from the live agent's
SQLite database (`arena-poker.sqlite3` on a remote host) — see
`docs/PLAN.md` for the export process. By default the console serves those
fixtures, with no live DB connection, no credentials, in this repo.

**Live bridge.** To run on fresh data instead of a stale export:

```
tools/sync_live.sh     # WAL-consistent snapshot → data/live/ (SSH alias only)
```

With `data/live/arena-poker.sqlite3` present, the server opens it
**read-only** (URI `mode=ro` + `PRAGMA query_only`) and re-checks the file
on every request — a new sync is picked up on the next page load, no
restart. The Quality Surface badge flips to LIVE and the broken-loop
callout reflects current counts. `SEKATS_DB=/path/to.db` points at an
explicit snapshot; `SEKATS_DB=off` forces fixture mode. The snapshot is
142 MB of private-host data and is git-ignored.

## Pluggable schema

The engine (`internal/engine`) is domain-neutral: decisions, shadow
reviews, declarative rules, drafts, and patches. Domain knowledge lives in
adapters (`internal/adapters`, default `poker`, selected via
`SEKATS_ADAPTER`): which policy sections resolve to checkable lists, what a
"repeated pattern" key is, and what the drafted edit says and where it
lands. Adding another agent with a policy file and a decision log is a new
adapter plus its policy and rules JSON — the promotion pipeline and UI are
untouched. The poker schema's SQL lives in `internal/live` and is the next
seam for a differently-shaped decision log.

## Run it

```
go build -o bin/server ./cmd/server
./bin/server
# http://localhost:8080
```

Override `ADDR`, `DATA_DIR`, `TEMPLATE_DIR`, `STATIC_DIR` via env vars if
needed. Other env vars:

- `SEKATS_DB` — explicit snapshot path (or `off` for fixture mode).
  Unset: auto-detect `data/live/arena-poker.sqlite3`.
- `SEKATS_ADAPTER` — domain adapter, default `poker`.
- `SEKATS_PROMOTE_THRESHOLD` — rejections per pattern before a policy edit
  is auto-drafted, default `3`.
- `SEKATS_ATTEST` — `off` disables on-chain anchoring (default: auto,
  signed mode requires `SEKATS_ATTEST_CONTRACT` and `SEKATS_APPROVERS`
  plus relay credentials: `SEKATS_KEYSTORE` + `SEKATS_KEYSTORE_PASS_FILE`
  on cast ≥1.5, or `SEKATS_ATTEST_KEY` on older cast).
- `SEKATS_APPROVERS` — comma-separated authorized human wallet addresses.
- `SEKATS_CHAIN_ID` — signing domain and relay chain ID (default `143`).
- `SEKATS_ATTEST_LEGACY=true` — explicitly use the old permissionless contract.
- `SEKATS_POLICY_FILE` — resulting policy file to hash read-only when marked applied.
- `SEKATS_WALLET_RPC` — public wallet chain-add RPC, separate from service RPC.
- `SEKATS_RPC` / `SEKATS_EXPLORER` / `CAST_BIN` — anchoring config.
- `SEKATS_AGENT_ADDRESS` — public agent wallet address for the Quality Surface
  treasury line. Reads `eth_getBalance` and `eth_getTransactionCount` through
  `SEKATS_RPC` (default `https://rpc.monad.xyz`), with a 3-second total timeout
  and a 15-second server-side cache for successes and failures. No wallet key
  is needed. Missing/invalid config shows `not configured`; failed reads show
  `RPC unavailable`, never a fabricated zero. Transaction count is the wallet's
  outgoing nonce, not its incoming transfers.
- `SEKATS_JURY_CONTRACT` — `SekatsJury` address enabling permissionless
  juror verdicts on `/review`; unset hides the buttons. When set, a
  background indexer also reconstructs the juror roster from `JurorVerdict`
  logs for `/api/jury` (cursor persisted to `data/jury-roster.json`).
- `SEKATS_JURY_FROM_BLOCK` — first block the juror indexer scans; pin near
  contract deployment. Default: recent-50000 lookback.
- `SEKATS_INDEX_RPC` — endpoint for the juror log scan, default
  `SEKATS_PUBLIC_RPC`. Public Monad RPCs cap `eth_getLogs` near 100 blocks;
  an indexer-grade endpoint (Envio HyperRPC, Alchemy) plus
  `SEKATS_LOG_CHUNK` (default `100`) converges far faster.
- `SEKATS_NANSEN_API_KEY` — Nansen Profiler key (`api.nansen.ai`). Enables
  wallet labels on the agent treasury line, anchor attesters, and the juror
  roster (`/api/wallet-labels`, `labels` in `/api/jury`,
  `attesterLabels` in `/api/anchors`). Unset: no labels render and APIs
  report `disabled` — nothing is fabricated. Optional offline fallback:
  `data/nansen-labels.json` snapshot (`{address: {labels: [...], as_of}}`)
  serves `source: "snapshot"` when live calls are unavailable.
- `SEKATS_PUBLIC_RPC` — public browser verification RPC, default
  `https://rpc.monad.xyz`. This URL is exposed in HTML, JSON and cast commands;
  use only a public, unauthenticated endpoint that allows browser CORS.
  It is separate from `SEKATS_RPC` to avoid exposing server RPC credentials.
  Registry reads also work with `SEKATS_ATTEST=off`; `SEKATS_ATTEST_CONTRACT`
  supplies a fallback contract for old records without an address.
- `SEKATS_DB_HOST` / `SEKATS_DB_REMOTE` — used by `tools/sync_live.sh`.
- `FLAGSMITH_API_TOKEN` (or `FLAGSMITH_API_KEY`) — organisation API token for the Flagsmith
  Management API. Create it in Organisation Settings → API Access → Create
  API Key. It stays server-side and is never rendered.
- `FLAGSMITH_PROJECT_ID` — numeric Flagsmith project id.
- `FLAGSMITH_ENVIRONMENT_KEY` — canonical environment key used in Management
  API paths and dashboard links.
- `FLAGSMITH_SERVER_ENVIRONMENT_KEY` — server-side environment key (normally
  beginning `ser.`) sent as `X-Environment-Key` for remote identity evaluation
  and experiment events. Server-key-only flags are intentionally hidden from
  client-key evaluations, so this separate key is required.
- `FLAGSMITH_ROLLOUT_PERCENT` — percentage of decision identities enrolled
  in each new experiment, default `20`. Use `100` for the small fixture demo;
  use a lower percentage for a real traffic cohort.
- `FLAGSMITH_CANDIDATE_PERCENT` — percentage of enrolled identities assigned
  to the candidate variant, default `50` (control receives the remainder).
- `FLAGSMITH_API_URL` / `FLAGSMITH_EVALUATION_API_URL` / `FLAGSMITH_EVENTS_URL` /
  `FLAGSMITH_DASHBOARD_URL` — optional overrides for self-hosting. Cloud defaults
  use the Management API at `api.flagsmith.com`, identity evaluation at
  `edge.api.flagsmith.com`, and event ingestion at `events.api.flagsmith.com`.
  When `FLAGSMITH_API_URL` is overridden, evaluation defaults to the same
  self-hosted service unless `FLAGSMITH_EVALUATION_API_URL` is set separately.

## Flagsmith graduated rollouts

Sekats uses the Flagsmith Management API directly rather than the MCP server
because experiment creation/start requires the API lifecycle. Minimal cloud
setup:

```
export FLAGSMITH_API_TOKEN=<organisation-api-token>
export FLAGSMITH_PROJECT_ID=<numeric-project-id>
export FLAGSMITH_ENVIRONMENT_KEY=<canonical-environment-key>
export FLAGSMITH_SERVER_ENVIRONMENT_KEY=<server-side-environment-key>
# optional for the tiny fixture demo: enroll the whole replay cohort
export FLAGSMITH_ROLLOUT_PERCENT=100
```

For local development the values can instead be kept in the git-ignored
`data/state/flagsmith.env` file and loaded into the server process explicitly
(the server does not auto-read dotenv files):

```
mkdir -p data/state
umask 077
cat > data/state/flagsmith.env <<'EOF'
FLAGSMITH_API_TOKEN=<organisation-api-token>
FLAGSMITH_PROJECT_ID=<numeric-project-id>
FLAGSMITH_ENVIRONMENT_KEY=<canonical-environment-key>
FLAGSMITH_SERVER_ENVIRONMENT_KEY=<server-side-environment-key>
FLAGSMITH_ROLLOUT_PERCENT=100
EOF
chmod 600 data/state/flagsmith.env
set -a; . data/state/flagsmith.env; set +a
# start the server from this same shell
```

Find the project id and canonical environment key in the Flagsmith
project/environment settings; create a server-side environment key in the
environment's SDK-key settings and the organisation token under Organisation
Settings → API Access. Keep all four values out of git. On a human approval the server:

1. ensures the environment has Flagsmith's zero-config managed warehouse;
2. creates or reuses a multivariate flag named `sekats_policy_<draft_id>`
   with flag value `control` and a keyed `candidate` variant;
3. creates metrics for mean human-verdict alignment and summed policy violations;
4. creates and starts a `sekats-<draft_id>` experiment with the configured
   rollout and variation split;
5. evaluates every human-verdicted disagreement as the deterministic identity
   `decision-<id>` (falling back to draft evidence), applies the candidate only
   inside the draft's pattern while leaving off-pattern behaviour at control,
   excludes unlabelled pending rows rather than counting them as failures,
   sends `$flag_exposure` plus conversion events to Flagsmith, and stores the
   resulting local audit rows in the adapter's rollout state file
   (`data/flagsmith_rollouts.json` for poker).

Event timestamps are the replay time, not the original decision timestamps:
this is a retrospective cohort measurement, not a reconstruction of live
serving chronology. The approved draft card shows variant-level replay counts,
the Flagsmith dashboard link, and win probability once the warehouse computation exists.
The home quality surface summarizes active experiments. If Flagsmith is not
configured or is unreachable, approval still succeeds and every unevaluated
identity is explicit control fallback; the card says so and exposes **Retry
rollout / refresh results**.

Flagsmith rebuilds edge environment documents asynchronously after an
experiment starts, and warehouse ingestion/results are asynchronous too. If the
first replay shows zero exposures, wait a few seconds and retry. While
`ExposureSent` is zero the retry re-evaluates the replay; after exposures have
been accepted it only refreshes results, avoiding a second conversion batch.
This protection depends on local state: if Flagsmith ingests a batch but the
response is lost, the server cannot know it succeeded. Wait and inspect the
dashboard after an ambiguous timeout rather than immediately repeating it.
Flagsmith's results-refresh endpoint is asynchronous and currently throttled
to about one refresh per five minutes, so repeat clicks can honestly return
the previous computation.

At the time of writing Flagsmith Cloud's free tier includes 50,000 requests
per month and up to 100,000 users in experiments, but experimentation is beta,
so check the current plan limits before a production-size replay. Flagsmith
also withholds inference below its data floor (currently at least 50 exposed
identities per variant; relative lift additionally requires a positive control
mean, and occurrence metrics have their own conversion minimums). A tiny
three-verdict fixture should honestly say "collecting data"; do not fabricate
extra traffic to force a win probability.

Retrospective human-verdict alignment is a validation signal, not proof of poker
profitability or downstream safety. The policy-violation metric is the
guardrail; losing or inconclusive experiments must remain visible rather than
being silently promoted.

Local state (`data/verdicts.json`, `data/pattern_rejections.json`,
`data/policy_drafts.json`, `data/flagsmith_rollouts.json`,
`data/attestations.json`, plus adapter-specific files under `data/state/`) is
git-ignored; deleting it resets the console to a clean slate. Keep
`data/state/flagsmith.env` if you created it for live credentials; it is also
git-ignored.

## Applying an approved patch

Approved drafts download from `/promotions/{id}/patch` as
`sekats-policy-patch/v1` JSON. Apply it wherever the live policy file
lives — the console never writes to agent files:

```
python3 tools/apply_policy_draft.py flop-bet-to-check.policy-patch.json \
    /path/to/poker-policy.json
```

The script appends the entry to the target array, is idempotent (a second
run is a no-op), and refuses unknown schemas.

## Human-signed approval and policy lineage (Monad)

The redesigned `contracts/SekatsAttestations.sol` is deployed at
[`0x771a99F24E7Cd81d0937edcb797138AAC228FDfE`](https://monadscan.com/address/0x771a99F24E7Cd81d0937edcb797138AAC228FDfE)
(Monad mainnet, chain 143). It removes permissionless anchoring:
the owner manages approvers, and `anchorWithSig` accepts only an authorized
EIP-712 `Approval(bytes32 draftKey,bytes32 docHash)` signature. First approval
wins. The event records both human signer and transaction relayer.

On `/promotions`, **Approve with wallet** displays the exact
`sekats-approval/v2` canonical document before signing, connects through vanilla
`window.ethereum`, switches/adds the configured Monad chain, and signs typed
data. The server rebuilds the document/hash, checks the recovered signer against
`SEKATS_APPROVERS`, then relays through the existing service configuration.
The typed-data GET never approves a draft; only a verified signature does.
Wallet approval requires HTTPS or localhost. No JavaScript build step is needed.

After an operator applies the downloaded patch externally, **Mark applied and
anchor policy** reads `SEKATS_POLICY_FILE`, checks that it contains the approved
entry, hashes exact resulting file bytes, and appends `anchorPolicy` history.
The submitting account must be the contract owner or an approver. This does
**not** prove that a running agent executes the file.

Without relay configuration, local approvals still work but carry no
cryptographic human-approval proof. For the previously deployed permissionless
contract
([`0x06bce5d59bf854d6e2900d71af3093270dc2c3bf`](https://monadscan.com/address/0x06bce5d59bf854d6e2900d71af3093270dc2c3bf)),
explicitly set `SEKATS_ATTEST_LEGACY=true`; it retains the old
`sekats-attestation/v1` service-only path, not wallet proof or policy lineage.
Do not use that flag against the redesigned contract.

See [docs/SIGNED_APPROVALS.md](docs/SIGNED_APPROVALS.md) for HTTP endpoints,
configuration, build-only deployment instructions, exact test commands, and the
registry worker's ABI migration. See [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md)
for what signatures and lineage prove, and what they do not. Keep operator
routes behind access control.

## Juror verdicts (permissionless)

`contracts/SekatsJury.sol` is the open counterpart to the gated attestation
registry, deployed at
[`0x0A8753aEE7684f8fB7773EfDD104B61C92fC9A14`](https://monadscan.com/address/0x0A8753aEE7684f8fB7773EfDD104B61C92fC9A14).
Anyone may record which side of an agent/shadow-LLM disagreement they endorse:
`attest(bytes32 decisionKey, uint8 verdict)` where `decisionKey` is
`sha256("sekats/decision/" + decisionID)`, `1` sides with the agent and `2`
with the shadow reviewer. `msg.sender` is the juror — the visitor's wallet
sends the transaction directly, so it works on a signer-free host and needs
no server configuration beyond the address.

Set `SEKATS_JURY_CONTRACT` to enable the **Side with agent / Side with
shadow** buttons under each review row; tallies are read back client-side
from `SEKATS_PUBLIC_RPC` (`tally(bytes32)`), so displayed counts are chain
truth, not server claims. A juror may change their vote; counts move.
Observing is open by design — promoting policy stays behind the
approver allowlist.

## Reuse the read-only verifier

Serve `static/js/anchor-verifier.js` unchanged from an HTTPS site or localhost.
Import its single function from a vanilla module script, with no build step:

```js
import { verifyAnchor } from "/static/js/anchor-verifier.js";
const result = await verifyAnchor({ draftID, docURL, contract, rpc });
```

`docURL` must serve the exact canonical attestation bytes. The module computes
SHA-256 over those bytes and over `"sekats/draft/" + draftID`, then calls only
`anchored(bytes32) returns (bytes32)` via `eth_call`. It ignores the server's
stored hash and key. Results are `match`, `mismatch`, `not-found` (zero hash or
no contract), or `unavailable` (document, RPC, CORS, timeout or crypto failure).
Confirm the contract address and RPC independently; this verifies document
integrity, not the registry's claimed attester or transaction. No additional
ABI, account access, signature or transaction is required.

Verifier tests run with `node --test static/js/anchor-verifier.test.mjs`.

## Kane CLI verification

See `docs/PLAN.md` for the closed-loop plan: Kane flows verify each of the
screens render their key numbers/callouts, that the review-queue
accept/reject action actually updates state and persists, that the
verdict report/export reflect recorded verdicts, that the data-source
badge is honest about LIVE vs FROZEN, and that rejecting a repeated
pattern auto-drafts a policy edit that a human can approve into a valid
patch.
