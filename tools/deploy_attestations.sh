#!/usr/bin/env bash
# Deploy SekatsAttestations to Monad (mainnet by default).
#
# Usage:
#   SEKATS_ATTEST_KEY=0x… tools/deploy_attestations.sh
#
# Env:
#   SEKATS_ATTEST_KEY  hex private key of the attester account (required).
#                      Use a dedicated low-balance key — it appears in
#                      shell env and cast argv. Never the agent wallet key.
#   SEKATS_RPC         default https://rpc.monad.xyz (Monad mainnet, 143)
#
# contracts/SekatsAttestations.bin is the committed creation bytecode,
# compiled once with solc 0.8.26 (--optimize). To rebuild:
#   npx solc@0.8.26 --optimize --bin contracts/SekatsAttestations.sol -o contracts/build
# Prints the deployed address — put it in SEKATS_ATTEST_CONTRACT.
set -euo pipefail
cd "$(dirname "$0")/.."

RPC="${SEKATS_RPC:-https://rpc.monad.xyz}"
BIN=contracts/SekatsAttestations.bin
: "${SEKATS_ATTEST_KEY:?set SEKATS_ATTEST_KEY to the attester private key}"

command -v cast >/dev/null || { echo "cast not found — install Foundry"; exit 1; }
[ -f "$BIN" ] || { echo "no bytecode at $BIN"; exit 1; }

echo "Attester: $(cast wallet address --private-key "$SEKATS_ATTEST_KEY")"
echo "Balance : $(cast balance "$(cast wallet address --private-key "$SEKATS_ATTEST_KEY")" --rpc-url "$RPC" --ether) MON"
echo "Deploying to $RPC …"

OUT=$(mktemp)
cast send --create "0x$(cat "$BIN")" \
  --rpc-url "$RPC" \
  --private-key "$SEKATS_ATTEST_KEY" \
  --json > "$OUT"

ADDR=$(python3 -c "import json; print(json.load(open('$OUT'))['contractAddress'])")
STATUS=$(python3 -c "import json; print(json.load(open('$OUT'))['status'])")
[ "$STATUS" = "0x1" ] || { echo "deploy failed"; cat "$OUT"; exit 1; }
rm -f "$OUT"

echo "SekatsAttestations deployed at $ADDR"
echo "Set SEKATS_ATTEST_CONTRACT=$ADDR and restart the console."
