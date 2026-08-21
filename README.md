# sekats

**stakes**, reversed — a parlor that watches whether a poker agent on
[DevFun Arena](https://arena.dev.fun/) is still the agent it claims to be.

The live runner is the *subject*. This repo is the *check*: a local QC cockpit
plus Kane CLI that treat `arena.dev.fun` as source of truth for anything the
world can see. Kane does not play poker. The probe uses **no API key**.

## Routes

| Path | What |
|------|------|
| `/` | Boot |
| `/sheet` | Five cards. **Public** is the canary for this parlor’s `contract.json`. |
| `/check` | Paste any public agent (URL, id, or live name). Field scan, or pin claimed season ids for **HOLD** / **DRIFT**. |
| `/about` | Plain-language read |

## Run

```
python3 tools/qc_server.py
```

Open http://127.0.0.1:8765/

```
./verify.sh
```

Runs the Kane `_test.md` suite (needs `kane-cli` + Chrome and the cockpit
listening), then the season probe. Probe exit 1 on **DRIFT** until
`assets/contract.json` matches live `list-active`.

Demo the closed loop:

```
./tools/demo_contract.sh stale   # Public → DRIFT
./tools/demo_contract.sh live    # Public → HOLD
```

Reload `/sheet` → Public after swapping.

Any-agent (CLI):

```
python3 tools/qc_server.py --probe-only --agent <id-or-name>
python3 tools/qc_server.py --probe-only --agent <id> --claimed <id,id>
```

## Layout

```
assets/                 # parlor UI + contract.json (+ stale/live for the tape)
tools/qc_server.py      # static server + credential-free /api/probe
tools/demo_contract.sh  # swap claimed seasons for DRIFT ↔ HOLD
docs/public-contract.md # plan + Kane invariants
tests/kane/             # Kane browser suite
verify.sh               # Kane then season probe
```

## Security

- Probe hits public discovery and public HTML only. No `x-arena-api-key`.
- Do not commit API keys, `.env`, wallet keys, or host inventory.
- The poker runner, VPS, and SQLite memory are a separate subject — not this app.

## Pre-commit

Hooks block private keys / secrets (gitleaks), fix trailing whitespace, and
lint/format the Python probe (ruff).

```
pip install pre-commit   # once
pre-commit install
pre-commit run --all-files
```

Plan: [`docs/public-contract.md`](docs/public-contract.md).
