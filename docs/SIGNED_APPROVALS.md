# Signed approvals: build, operator setup, and ABI

This is a **new deployment**, not an upgrade of the permissionless contract.
The old committed creation bytecode was removed to prevent deploying it by
mistake. No deployment is performed by the build helper.

> Deployed 2026-10-13 to Monad mainnet at
> [`0x771a99F24E7Cd81d0937edcb797138AAC228FDfE`](https://monadscan.com/address/0x771a99F24E7Cd81d0937edcb797138AAC228FDfE)
> (deploy tx `0xf5113a6b…80cd`), compiled with `solc 0.8.26 --optimize` and
> sent via `cast send --create` with the constructor arg ABI-appended
> (owner = the dedicated attester wallet). The old contract
> (`0x06bce5d5…c3bf`) remains valid under `SEKATS_ATTEST_LEGACY=true`.

## Offline/local validation

From the repository root (Go 1.26.2, Foundry, Solidity 0.8.26):

```sh
go build ./... && go vet ./... && go test ./...
go test -race ./internal/attest ./internal/promotion ./cmd/server
forge build
forge test
node --check static/js/wallet-approval.js
bash tools/test_wallet_ui.sh  # agent-browser + Chromium + Python 3
```

Foundry tests are self-contained, with no `forge-std` dependency. They need no
wallet, private key, RPC, or funds. If solc is installed explicitly:

```sh
forge build --use /absolute/path/to/solc-0.8.26 --offline
forge test --use /absolute/path/to/solc-0.8.26 --offline
```

If Foundry is unavailable, compile the contract and tests with solc (compilation
alone does not execute the Foundry cheatcodes):

```sh
solc --optimize --abi --bin --base-path . \
  contracts/SekatsAttestations.sol contracts/test/SekatsAttestations.t.sol \
  -o /tmp/sekats-solc-build
# Or: npx solc@0.8.26 --optimize --abi --bin --base-path . \
#   contracts/SekatsAttestations.sol contracts/test/SekatsAttestations.t.sol \
#   -o /tmp/sekats-solc-build
```

Generated build artifacts stay ignored. `contracts/SekatsAttestations.abi.json`
contains the redesigned contract ABI for other workers.

## Deployment instructions only

`bash tools/deploy_attestations.sh` builds locally and prints instructions.
It never connects to an RPC, reads a key, checks a balance, or sends a transaction.

An operator must separately select the network and owner, review the contract,
and use an existing secure signing arrangement. The deployment command template
is in that helper; no credentials are supplied here. After deployment, the owner
calls `setApprover(humanWallet, true)` for each authorized EOA. Keep the local
`SEKATS_APPROVERS` allowlist in sync. The owner is not automatically an approver.
The default contract constructor takes exactly one address, the immutable owner.

## Server configuration

For an operator's **existing** relay configuration, set:

| Setting | Purpose |
| --- | --- |
| `SEKATS_ATTEST_KEY` | Existing service relay credential, configured outside the repo; never a human wallet credential. Passed to cast as `ETH_PRIVATE_KEY`, which cast versions ≥1.5 no longer honor — use the keystore settings below instead |
| `SEKATS_KEYSTORE` | Path to an encrypted keystore file (e.g. `cast wallet import`); preferred relay credential on cast ≥1.5 — keeps key material out of argv and env |
| `SEKATS_KEYSTORE_PASS_FILE` | Path to the keystore password file, required with `SEKATS_KEYSTORE` |
| `SEKATS_ATTEST_CONTRACT` | Newly deployed contract address |
| `SEKATS_APPROVERS` | Comma-separated authorized human wallet addresses; required in signed mode |
| `SEKATS_CHAIN_ID` | Domain/transaction chain ID, default `143` |
| `SEKATS_RPC` | Service RPC URL; default `https://rpc.monad.xyz` |
| `SEKATS_WALLET_RPC` | Public URL for wallet chain addition; default `https://rpc.monad.xyz` |
| `SEKATS_EXPLORER` | Explorer base URL; default `https://monadscan.com` |
| `SEKATS_POLICY_FILE` | Optional operator-configured resulting policy file path, read-only |
| `CAST_BIN` | Foundry cast executable |
| `SEKATS_ATTEST` | `off` disables all relay activity |
| `SEKATS_ATTEST_LEGACY` | Only literal `true` enables the old contract path |

Use a new `DATA_DIR` populated with the required fixtures for the new deployment,
or explicitly reconcile existing state. Do not treat an old contract's records as
anchors on the new one. No new deployment address is claimed by this branch.
Serve over HTTPS (localhost is allowed for local tests) so Web Crypto is available.
Behind a TLS proxy, preserve the original Host and forward HTTPS to the server,
or configure the proxy to remove Origin only after enforcing its own same-origin
and authentication checks. The server does not blindly trust forwarded headers.

## Approval HTTP flow

1. `GET /promotions/{id}/typed-data` returns `document` (the exact JSON string),
   `doc_sha256`, `typed_data` (EIP-712), and `chain` (wallet chain-add metadata).
   It does not approve the draft. Pending and locally approved drafts may sign;
   discarded or already anchored drafts are rejected.
2. The wallet UI displays the document, checks its SHA-256 with Web Crypto,
   requires review consent, connects with `window.ethereum`, switches/adds
   the configured Monad chain, and calls `eth_signTypedData_v4`.
3. `POST /promotions/{id}/signature` accepts **only**
   `{"signature":"0x<65-byte signature>"}`. The server rebuilds the document,
   recovers the address, checks its configured allowlist, persists approval,
   and relays `anchorWithSig`. Invalid signatures do not approve drafts.
   Relay failure leaves a valid locally approved draft with a failed record;
   the human can sign the same document again to retry. A successful response
   includes `status`, `attester`, `tx_hash`, and `tx_url`.
4. `GET /promotions/{id}/attestation` returns exactly the anchored preimage,
   without a trailing newline. `X-Sekats-Doc-SHA256` supplies its digest.
   Patch downloads and the existing apply script remain separate actions.
5. After applying the patch externally, `POST /promotions/{id}/applied` with
   `{}` reads **only** `SEKATS_POLICY_FILE`, checks the approved entry, hashes raw
   bytes, and relays `anchorPolicy`. It never applies the patch itself or accepts
   a client file path/hash. The relay account must be an on-chain owner or
   approver. An identical last anchored file hash is a local no-op; later changed
   bytes append history. UI exposes “Mark applied and anchor policy” when configured.

Existing `POST .../approve` and `POST .../attest` are blocked in signed mode.
They remain available for local/offline approval or explicit legacy mode.
Keep operator actions behind access control; same-origin checks are not login.
See [THREAT_MODEL.md](THREAT_MODEL.md) for authority and execution-proof limits.

## Registry worker ABI summary

Domain: `{name:"SekatsAttestations", version:"2", chainId, verifyingContract}`.
Primary type: `Approval(bytes32 draftKey,bytes32 docHash)`.
`draftKey = SHA-256("sekats/draft/" + draftID)`;
`docHash = SHA-256(canonical document bytes)`.
`policyHash = SHA-256(exact resulting policy-file bytes)`.

### Transactions

- `constructor(address initialOwner)`
- `setApprover(address approver, bool allowed)` (owner only)
- `anchorWithSig(bytes32 draftKey, bytes32 docHash, bytes signature)`
  (any relayer, currently authorized recovered signer)
- `anchorPolicy(bytes32 draftKey, bytes32 policyHash)` (owner or approver,
  only after draft approval, zero-based append-only history)

### Reads

- `anchored(bytes32) returns (bytes32)` remains compatible.
- `attesters(bytes32) returns (address)` returns the **human signer**.
- `isApprover(address) returns (bool)`
- `owner() returns (address)`
- `policyCount(bytes32) returns (uint256)`
- `policyHistory(bytes32,uint256) returns (bytes32)`
- `domainSeparator() returns (bytes32)`
- `approvalDigest(bytes32,bytes32) returns (bytes32)`
- `APPROVAL_TYPEHASH() returns (bytes32)`

### Events

```solidity
ApproverSet(address indexed approver, bool allowed)
Anchored(bytes32 indexed draftKey, bytes32 indexed docHash,
         address indexed attester, address relayer)
PolicyAnchored(bytes32 indexed draftKey, bytes32 indexed policyHash,
               uint256 index, address indexed attester)
```

**Breaking:** there is no `anchor(bytes32,bytes32)` on this deployment.
The `Anchored` event topic signature changes to
`Anchored(bytes32,bytes32,address,address)`; update event decoding, not just the
getter. Policy `attester` is the submitting operator, not necessarily the original
approval signer. Registry handlers/templates and treasury content are deliberately
untouched by this branch.
