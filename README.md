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
disagrees with itself, gives a human a real queue to adjudicate those
disagreements, and — closing the loop that produced the 0.1% promotion
rate — auto-drafts a policy edit when a human keeps rejecting the agent on
a repeated pattern, stages it for review, and produces an applyable patch
on approval.

Built in Go with server-rendered HTML + htmx — no JS build step, no
framework magic, on purpose. See `docs/PLAN.md` for the full rationale.

## Screens

1. **Quality Surface** (`/`) — the broken-loop callout, shadow-LLM
   agreement rate, review-provider error buckets, and a data-source badge
   stating whether the numbers are LIVE (read-only bridge) or FROZEN
   (stale export) — the callout fires on fresh rot, not a stale export.
2. **Policy-as-ACs** (`/policy`) — the policy file's hand-tier rules, run
   as assertions against real logged decisions. The rules are declarative
   JSON (`data/poker-rules.json`), not Go code. Currently catches 1 real
   violation in the sample.
3. **Review Queue** (`/review`) — 25 real hands where the agent and the
   shadow-LLM disagreed, with accept/reject buttons (htmx partial swap).
   Verdicts persist to disk and survive a restart. Rejecting a hand also
   feeds the pattern buckets below.
4. **Promotion Pipeline** (`/promotions`) — the closed loop: rejections on
   a repeated pattern (default: 3 per pattern) auto-draft a policy edit
   into `postflop.promotedFromHandReviews` (the same section the agent's
   existing promotions live in), with the rejected hands as evidence. A
   human approves (→ downloadable `.policy-patch.json`) or discards.
   Nothing is ever applied automatically. Approvals can additionally be
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
  signed mode requires `SEKATS_ATTEST_KEY`, `SEKATS_ATTEST_CONTRACT`,
  and `SEKATS_APPROVERS`).
- `SEKATS_APPROVERS` — comma-separated authorized human wallet addresses.
- `SEKATS_CHAIN_ID` — signing domain and relay chain ID (default `143`).
- `SEKATS_ATTEST_LEGACY=true` — explicitly use the old permissionless contract.
- `SEKATS_POLICY_FILE` — resulting policy file to hash read-only when marked applied.
- `SEKATS_WALLET_RPC` — public wallet chain-add RPC, separate from service RPC.
- `SEKATS_RPC` / `SEKATS_EXPLORER` / `CAST_BIN` — anchoring config.
- `SEKATS_DB_HOST` / `SEKATS_DB_REMOTE` — used by `tools/sync_live.sh`.

Local state (`data/verdicts.json`, `data/pattern_rejections.json`,
`data/policy_drafts.json`, `data/attestations.json`) is git-ignored;
deleting it resets the console to a clean slate.

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

The redesigned `contracts/SekatsAttestations.sol` requires a **new deployment**.
No deployment was performed for this change. It removes permissionless anchoring:
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
contract, explicitly set `SEKATS_ATTEST_LEGACY=true`; it retains the old
`sekats-attestation/v1` service-only path, not wallet proof or policy lineage.
Do not use that flag against the redesigned contract.

See [docs/SIGNED_APPROVALS.md](docs/SIGNED_APPROVALS.md) for HTTP endpoints,
configuration, build-only deployment instructions, exact test commands, and the
registry worker's ABI migration. See [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md)
for what signatures and lineage prove, and what they do not. Keep operator
routes behind access control.

## Kane CLI verification

See `docs/PLAN.md` for the closed-loop plan: Kane flows verify each of the
screens render their key numbers/callouts, that the review-queue
accept/reject action actually updates state and persists, that the
verdict report/export reflect recorded verdicts, that the data-source
badge is honest about LIVE vs FROZEN, and that rejecting a repeated
pattern auto-drafts a policy edit that a human can approve into a valid
patch.
