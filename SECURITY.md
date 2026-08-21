# Security

This repository is a **credential-free** public canary. It must stay that way.

## Allowed

- Public Arena URLs and agent ids (`arena.dev.fun/agent/…`)
- Claimed competition ids in `assets/contract.json` (already public on the leaderboard)
- Local probe against `list-active` and public profile HTML

## Never commit or paste

| Class | Examples |
|-------|----------|
| API keys | `.arena-credentials`, `arena_sk_…`, `x-arena-api-key` values |
| Env / LLM | `.env`, provider tokens, `LLM_PROVIDER_ORDER` secrets |
| Host inventory | VPS hostnames, IPs, SSH aliases, key paths, `scp` recipes |
| Wallet | Private keys, seed phrases, funded-wallet drain steps |
| Private ops | `AGENTS.md`-style runbooks, `ARCHIVE.md`, SQLite dumps |
| Kane traces | `.testmuai/`, `tests/kane/**/output-*/` |

## Enforcement

- `.gitignore` blocks credentials, env, SQLite, Kane session dirs.
- `pre-commit` runs **gitleaks** + **detect-private-key** + ruff on every commit.
  ```
  pre-commit install
  pre-commit run --all-files
  ```
- The probe (`tools/qc_server.py`) must not grow a path that reads `.arena-credentials`.

## Publishing from the private agent workspace

The poker agent repo is a **different** tree. It contains host inventory and
credentials tooling. Do **not** `git push` that repository to this remote.

The live parlor (https://sekats.thisyearnofear.com) is a **separate host** from
the poker runner. Only this credential-free tree is deployed there.

To update sekats:

1. Copy only canary paths (see `docs/PUBLISH.md`).
2. Run `pre-commit run --all-files`.
3. Push from the publish tree, not from the agent workspace.
4. Deploy the publish tree to the parlor host (private ops runbook) and restart
   the parlor service — never onto the machine that holds API keys / SQLite.

If a secret ever lands in git history, rotate the key and treat the commit as
compromised — force-push alone is not enough for a public fork.
