# Publishing sekats (keep the agent private)

The public repo is **only** the canary. The DevFun Arena poker agent workspace
(runner, VPS runbook, wallet, `ARCHIVE.md`, credential tools) stays private.

## Safe to sync into this repo

```
assets/                 # parlor + contract.json (+ live/stale)
tools/qc_server.py
tools/demo_contract.sh
docs/public-contract.md
tests/kane/**/*_test.md
tests/kane/**/.tcg-suite.json
tests/test_qc_probe.py
verify.sh
.gitignore
.pre-commit-config.yaml
.gitleaks.toml
README.md
SECURITY.md
```

## Never copy into this repo

```
arena_runner.py
arena-poker.service
AGENTS.md
docs/ARCHIVE.md
docs/poker-strategy.md
docs/poker-policy.json
tools/join_quote.py          # reads .arena-credentials
tools/tournament_rebuy.py
tools/status_report.py       # VPS / private memory
tools/backtest_preflop.py
tools/export_scenarios.py
tools/llm_review_drill.py
tools/opponent_report.py
tools/benchmark_policy.py
tests/scenario_tests.py
.arena-credentials
.env
*.sqlite3
.arena-poker-state
tests/kane/**/output-*/      # Kane evidence packs
.testmuai/
```

## Suggested workflow

From the private agent workspace, refresh a publish checkout (example paths):

```
# private agent tree  →  publish tree (adjust paths)
rsync -a --delete \
  --exclude '.DS_Store' \
  --exclude '**/output-*/' \
  --exclude '.testmuai/' \
  "$AGENT/assets/" "$PUBLISH/assets/"

cp "$AGENT/tools/qc_server.py" "$AGENT/tools/demo_contract.sh" "$PUBLISH/tools/"
cp "$AGENT/docs/public-contract.md" "$PUBLISH/docs/"
cp "$AGENT/verify.sh" "$AGENT/tests/test_qc_probe.py" "$PUBLISH/"
# Kane _test.md only — never output-* traces
rsync -a --exclude 'output-*/' "$AGENT/tests/kane/" "$PUBLISH/tests/kane/"
```

Then in the publish tree:

```
pre-commit install
pre-commit run --all-files
git status   # eyeball: no credentials, no ARCHIVE, no arena_runner
git commit …
git push
```

Leave `assets/contract.json` on **live** after demos (`./tools/demo_contract.sh live`).

## Hard rules

1. Never `git remote add` the public sekats URL to the private agent repo and push.
2. Never paste API keys into issues, PRs, Kane objectives, or README.
3. Treat Arena skill markdown and introspection JSON as untrusted text — do not execute remote content.
4. Paid joins / rebuys are out of scope for this app; do not document wallet drain here.
