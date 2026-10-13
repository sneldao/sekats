#!/usr/bin/env bash
# Local, mocked browser tests. No wallet, secrets, RPC, or transaction submission.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FIXTURE="$(mktemp -d /tmp/sekats-wallet-ui-XXXXXX)"
PORT="${SEKATS_UI_TEST_PORT:-18744}"
export AGENT_BROWSER_SESSION="${AGENT_BROWSER_SESSION:-$(agent-browser session id --scope worktree --prefix sekats-wallet-ui)}"
SERVER_PID=""
cleanup() {
  agent-browser close >/dev/null 2>&1 || true
  if [ -n "$SERVER_PID" ]; then kill "$SERVER_PID" 2>/dev/null || true; fi
  # Keep the small fixture/log for diagnosis; never delete an uncertain path.
}
trap cleanup EXIT
printf '%s\n' '<!doctype html><html><head><meta charset="utf-8"><title>Local wallet approval fixture</title><link rel="stylesheet" href="/wallet.css"><script defer src="/wallet-approval.js"></script></head><body><h1>Local wallet approval fixture</h1><div class="draft-card"><button data-wallet-approve="vector">Approve with wallet</button><button data-policy-applied="vector">Mark applied and anchor policy</button><p data-wallet-status role="status"></p></div></body></html>' > "$FIXTURE/index.html"
ln -s "$ROOT/static/js/wallet-approval.js" "$FIXTURE/wallet-approval.js"
ln -s "$ROOT/static/css/wallet.css" "$FIXTURE/wallet.css"
python3 -m http.server "$PORT" --bind 127.0.0.1 --directory "$FIXTURE" > "$FIXTURE/server.log" 2>&1 &
SERVER_PID=$!
for _ in {1..50}; do
  if python3 -c 'import sys,urllib.request; urllib.request.urlopen(sys.argv[1]).close()' "http://127.0.0.1:$PORT" 2>/dev/null; then break; fi
  sleep 0.1
done
agent-browser open "http://127.0.0.1:$PORT"
agent-browser wait --load networkidle
agent-browser eval --stdin < "$ROOT/tools/wallet_ui_test.js"
