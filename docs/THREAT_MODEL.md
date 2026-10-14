# Signed approvals and policy lineage

## What is proven

- An address authorized by the contract's owner at execution time signed
  `Approval(bytes32 draftKey,bytes32 docHash)` for this EIP-712 domain:
  name `SekatsAttestations`, version `2`, chain ID, and verifying contract.
- `docHash` is SHA-256 of the exact canonical document bytes. The document
  contains the draft ID, pattern key, target, entry, sorted source decision IDs,
  and draft creation time. The server rebuilds it, never accepting a client
  document or hash. It has schema `sekats-approval/v2`; it deliberately excludes
  a future approval time, so fetching signing data does not approve anything.
- The first authorized approval of a draft key wins. The stored attester is
  the recovered **signer**, not the service that relayed it. Anyone may relay
  an existing valid signature, including a front-runner, but cannot change its
  approved hash. Guessing a draft ID alone cannot squat it.
- Policy history records a sequence of exact resulting-file SHA-256 hashes
  submitted by an approver or the owner **after** the draft was anchored.
  Each `PolicyAnchored` event identifies the submitting operator and index.
  The console checks that the approved entry exists at its target in the
  configured policy file, then hashes its raw bytes, not normalized JSON.

The precise claim is **“this approver key signed this docHash”** and
**“these policy hashes form lineage as anchored by authorized operators.”**
Off-chain records are convenient caches, not the source of chain authority.

## What is NOT proven

**Nothing here proves that the running agent actually executes the anchored
policy.** There is no runtime attestation, measured boot, execution trace,
enforcement hook, or remote verification of the agent's loaded policy.

Also not proven:

- That a key belongs to a particular human, that a human rather than automation
  used it, or that the signer understood the document. Wallets typically display
  the hashes. The UI shows the document and requires review consent, but a
  compromised server/browser can deceive a human.
- That evidence decision IDs, timestamps, policy edits, or outcomes are true,
  safe, complete, or representative. Signed metadata is not independently
  verified evidence.
- That the resulting file is the live agent's file rather than a snapshot.
  Applying an edit remains a separate operator action; the console only reads
  and reports the configured file. It does not write to the agent.
- That a policy history entry is intrinsically valid. The contract cannot
  inspect files; authorized operators may submit arbitrary hashes, duplicates,
  or false lineage. It only enforces authorization, ordering, and append-only
  storage, not parent-policy relationships.
- That revocation retroactively invalidates an existing anchor. Revocation
  prevents future uses of that approver's signatures, not historical approvals.

## Trust boundaries and residual risks

1. **Owner / allowlists.** The owner controls `isApprover` and may anchor policy.
   Compromise lets it authorize malicious keys or fabricate lineage. Owner is
   immutable; deploy with an appropriately protected address (for example a
   multisig). Losing it prevents allowlist administration. Malicious authorized
   approvers can lock draft keys with their own signed hashes; protection is
   against *unauthorized* squatting, not privileged sabotage.
2. **Relay account.** A non-approver relayer cannot fabricate human approvals.
   To use the direct `anchorPolicy` relay, it must be the owner or an approver.
   Prefer keeping it out of `isApprover`; giving it owner privileges still makes
   it powerful enough to grant itself approval rights. An alternative operational
   arrangement is a separate authorized operator calling `anchorPolicy` after
   application. This implementation does not provide policy signatures.
3. **Local configuration.** `SEKATS_APPROVERS` is checked before any approval
   mutation or relay; the on-chain allowlist is checked independently by the
   contract and wins if they differ. Incorrect chain/contract configuration
   makes signatures invalid. RPC chain ID and post-receipt hash/signer/history
   read-backs are checked. A malicious RPC can lie; independently verify receipts
   and contract code with a trusted provider.
4. **Replay.** Signatures bind chain ID and contract address; a different chain
   ID or deployment rejects replay. Forks that share both chain ID and contract
   address are not distinguished. Within a deployment, the draft key is the
   replay nonce and first approval wins. Signatures have no expiry; revoke an
   approver to invalidate its unused signatures.
5. **Namespace.** Draft keys are `SHA-256("sekats/draft/" + draftID)`. Different
   installations with the same IDs and contract share a namespace. Use a
   separate deployment/state directory for separate tenants or new versions.
6. **Server and access control.** There is no built-in operator login. Keep the
   console behind access control; do not expose relay, discard, review, or
   applied-reporting routes publicly. Same-origin checks reject cross-origin
   browser requests but are not authentication, rate limiting, or protection
   against non-browser callers. A stolen signature can be relayed for its
   authorized hash, consuming the service's gas.
7. **Availability / persistence.** Signed approval records are written before
   relaying, with atomic replacement and file sync. Approval and relay are not
   one distributed transaction. A timeout, crash after mining, reorg, failed
   local write, or concurrent external policy submission may leave a `failed`
   record despite a mined transaction. Reconcile contract state and receipts
   before retrying; a mined first approval cannot be overwritten. Keep backups
   of drafts, canonical documents, and records; hashes alone do not restore
   preimages. Run one writer against these file stores.
8. **Secrets.** Existing operator relay key configuration is used only at
   runtime. It is not written to state or subprocess arguments; cast receives
   it through `ETH_PRIVATE_KEY`. Environment/process access is still sensitive.
   Subprocess stderr and raw output are not returned as diagnostics. Browser
   chain-add metadata uses a separate public `SEKATS_WALLET_RPC`, not a possibly
   credentialed service RPC URL. `SEKATS_NANSEN_API_KEY` is likewise
   server-side only: it authorizes outbound calls to api.nansen.ai and must
   never appear in HTML, JS, API responses, or logs — browsers see labels,
   never the key.
9. **Signature scope.** Only 65-byte, low-S secp256k1 EOA signatures are supported,
   with recovery IDs 0/1 or 27/28. EIP-1271 contract-wallet signatures and EIP-2098
   compact signatures are not supported. Document SHA-256 and EIP-712 Keccak-256
   serve different purposes; do not interchange them.

## Legacy mode

`SEKATS_ATTEST_LEGACY=true` intentionally uses the **old deployed contract's**
`anchor(bytes32,bytes32)` path and `sekats-attestation/v1` document.
It cannot prove human wallet approval, prevents neither unauthorized squatting
nor service impersonation, and has no policy lineage. Its UI labels local/legacy
approval separately. The redesigned contract has **no** permissionless `anchor`
function. Do not enable the legacy flag against it.

## Validation boundaries

Tests use public signature components recovered locally, never private keys or
funded wallets. Foundry executes only its isolated local EVM. Browser tests mock
both EIP-1193 and HTTP relay responses. These validate encoding and failure paths,
not a real MetaMask extension, remote Monad RPC, production access control,
deployment, or finality. No contract was deployed and no mainnet/testnet RPC was
contacted as part of this work.
