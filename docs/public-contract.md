# Public-contract canary

The live runner is the *subject*. This surface is the *check*: a local QC
cockpit plus Kane CLI that treat `arena.dev.fun` as source of truth for
anything the world can see, and refuse to let the operator view disagree.

Kane does not play poker. The action clock is tens of seconds; Kane is a
real browser. Putting it on `POST /texas/action` is the wrong coupling.
The fertile surface is the **published** agent — profile, leaderboard,
delayed replay, live competition list — which other operators, other
agents, and hackathon judges can open with no API key.

## Why this exists

[dev.fun Arena](https://docs.dev.fun/) is a live market of API-only poker
agents (hundreds on playground/tournament seasons, a public $ prize pool).
The documented user is “paste a skill, don’t write code.” Those agents
have the same shape:

1. Hands-off API loop (`pending-actions` → `action` + public chat `message`).
2. A public page (`/agent/{id}`), leaderboards, delayed hand replay.
3. Almost no operator UI, and **no check** that the published agent matches
   what the process thinks it is doing.

We already pay for that gap: silent season rotation (`competition_not_active`),
public chat that must not leak hole cards, local memory that drifts from
the board. The parlor dashboard was a picture of ops. The canary polices
the public contract.

## What is in scope

- QC cockpit: `/` (boot) → `/sheet` (cards) → `/check` (any agent) → `/about`.
  Files: `assets/landing.html`, `assets/sheet.html`, `assets/check.html`,
  `assets/about.html`.
- **Public** channel: claimed identity + claimed seasons vs live
  `GET /api/arena/competition/list-active`.
- `assets/contract.json` — this parlor’s claimed public contract (handle,
  profile URL, competition IDs the process still believes). Kane still
  walks this file, not a pasted agent.
- `/check` — paste any public `arena.dev.fun/agent/<id>` (or live name).
  Optional claimed competition IDs → **HOLD** / **DRIFT**. Id/name only →
  **SCAN** (seated on live seasons). Same credential-free probe.
- `tools/qc_server.py` — one-command local server; static assets + a
  **credential-free** live probe. Does not read API keys.
  `GET /api/probe?agent=<id>&claimed=<id,id>` is the machine interface.
- Kane suite + `./verify.sh` — agent runs Kane, reads a fail, patches,
  runs again. Kane never sits in the decision loop.

The live poker runner, its host, SQLite memory, and wallet stay the subject.
They are not this app. See [`SECURITY.md`](../SECURITY.md) and
[`PUBLISH.md`](PUBLISH.md).

## What is out of scope

- Kane as fold/call/raise. Too slow, burns credits, wrong tool.
- 402 / rebuy / wallet flows in the demo.
- Handing judges API keys or private operator hosts.
- Shipping private ops docs, ARCHIVE, or credential-bearing tools.

## Invariants Kane must prove

1. **Boot contract** — “Enter the operations” reaches Stack Cartographer QC
   with Field selected and visible.
2. **Season contract** — every competition ID in `contract.json` appears in
   live `list-active`. Fail = **DRIFT**. Pass = **HOLD**.
3. **Identity contract** — cockpit handle/name match the live profile at
   `https://arena.dev.fun/agent/<id>` (title contains the agent name).
4. **Chat contract** (optional saved test, not the cold-open) — delayed public
   replay messages contain no hole-card tokens. Timing is awkward (one-hand
   delay), so it is a saved `_test.md`, not the opening shot.

## Closed loop (what we film)

Keep `assets/contract.stale.json` (old seasons) and `assets/contract.live.json`
(current). Default `contract.json` is live.

Tape, in order:

1. `python3 tools/qc_server.py` — landing → Enter the operations → **Public**.
2. `./tools/demo_contract.sh stale` — reload Public — **DRIFT**. Kane Public
   test still passes (it accepts DRIFT or HOLD). `./verify.sh` fails on the
   season probe.
3. `./tools/demo_contract.sh live` — reload — **HOLD**. Leave the contract on
   **live** after the tape.
4. Optional: `/check` with another public agent (field utility).

Do not open Chrome to check. Reload the cockpit; run `./verify.sh`.
Do not auto-pay tournament entry for the tape.

## How to run the cockpit

```
python3 tools/qc_server.py
```

Opens the parlor on `http://127.0.0.1:8765/`. Primary flow: boot →
`/sheet` → Public (key `5`, or swipe). `/check` is the any-agent paste.
`/about` is the plain-language read. The live URL judges can open without
us is the public profile in `contract.json`.

```
python3 tools/qc_server.py --probe-only --agent <id>
python3 tools/qc_server.py --probe-only --agent <id> --claimed <id,id>
```

```
./verify.sh
```

Runs the Kane `_test.md` suite under `tests/kane/` (needs `kane-cli` + Chrome,
and `qc_server.py` already listening), then the season probe. Probe exit 1 on
**DRIFT** is the production fail: patch `assets/contract.json` until it
reports **HOLD**.

## Security

- Probe uses public discovery and public HTML only. No `x-arena-api-key`.
- Never commit API keys, `.env`, wallet material, host inventory, or Kane
  `output-*` traces.
- `pre-commit` (gitleaks + private-key detect + ruff) is required before push.
- Publishing rules: [`PUBLISH.md`](PUBLISH.md). Policy summary: [`SECURITY.md`](../SECURITY.md).
