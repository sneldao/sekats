#!/usr/bin/env bash
# Build-only helper. Prints a deployment template; NEVER submits transactions.
set -euo pipefail
cd "$(dirname "$0")/.."
command -v forge >/dev/null || { echo "Install Foundry to build; see docs/SIGNED_APPROVALS.md."; exit 1; }
forge build "$@"
printf '%s\n' \
  "Build complete. Nothing was deployed; no RPC or signing key was used." \
  "Review docs/THREAT_MODEL.md and choose an owner and network separately." \
  "" \
  "Deployment template (operator action only, NOT executed by this script):" \
  "forge create contracts/SekatsAttestations.sol:SekatsAttestations \\" \
  "  --constructor-args <owner-address> --rpc-url <explicit-rpc-url> \\" \
  "  --account <existing-keystore-account> --broadcast" \
  "" \
  "After deployment the owner calls setApprover(address,bool) for human wallets." \
  "Use the new address with signed mode, not SEKATS_ATTEST_LEGACY=true." \
  "ABI: contracts/SekatsAttestations.abi.json"
