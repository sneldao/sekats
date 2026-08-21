#!/usr/bin/env bash
# Swap the cockpit's claimed contract for the video loop.
#   ./tools/demo_contract.sh stale   → Public shows DRIFT (S6/S7)
#   ./tools/demo_contract.sh live    → Public shows HOLD (S14/S15)
# Reload http://127.0.0.1:8765/sheet after swapping.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/assets"
case "${1:-}" in
  stale)
    cp "$SRC/contract.stale.json" "$SRC/contract.json"
    echo "claimed → stale (expect DRIFT). reload Public."
    ;;
  live)
    cp "$SRC/contract.live.json" "$SRC/contract.json"
    echo "claimed → live (expect HOLD). reload Public."
    ;;
  *)
    echo "usage: $0 stale|live" >&2
    exit 2
    ;;
esac
python3 "$ROOT/tools/qc_server.py" --probe-only | python3 -c 'import json,sys; d=json.load(sys.stdin); print("verdict", d.get("verdict"), "seat", d.get("seatVerdict"))'
