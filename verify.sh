#!/usr/bin/env bash
# Public-contract canary. The agent runs this after cockpit/contract edits.
# Kane suite first (cockpit must work). Season probe last: DRIFT is a real fail
# until contract.json matches live list-active.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

KANE_DIR="$ROOT/tests/kane"
if command -v kane-cli >/dev/null 2>&1; then
  KANE_TESTS=()
  while IFS= read -r t; do
    KANE_TESTS+=("$t")
  done < <(find "$KANE_DIR" -name '*_test.md' 2>/dev/null | sort)
  if ((${#KANE_TESTS[@]})); then
    echo "=== Kane browser suite ==="
    export KANE_CLI_USER_AGENT="${KANE_CLI_USER_AGENT:-cursor}"
    for t in "${KANE_TESTS[@]}"; do
      echo "running ${t#$ROOT/}"
      kane-cli testmd run "$t" --agent --headless --url http://127.0.0.1:8765/
    done
  else
    echo "no Kane _test.md files yet under tests/kane"
  fi
else
  echo "kane-cli not on PATH; skipping browser suite"
fi

echo "=== season probe (claimed vs live list-active) ==="
PROBE="$(python3 tools/qc_server.py --probe-only)"
export PROBE
python3 - <<'PY'
import json, os, sys
probe = json.loads(os.environ["PROBE"])
verdict = probe.get("verdict")
print("verdict:", verdict)
print("seat:", probe.get("seatVerdict"))
print("fetched:", probe.get("fetchedAt"))
missing = probe.get("missing") or []
if missing:
    print("missing from live:")
    for row in missing:
        print("  -", row.get("name"), row.get("id"))
print("live seasons:")
for row in probe.get("live") or []:
    print("  -", row.get("name"), row.get("id"))
if verdict != "HOLD":
    sys.exit(1)
PY
