---
mode: testing
url: http://localhost:8080/promotions
max_steps: 60
tags: [promotion-pipeline, attestation, monad, smoke]
---

# An approved policy edit carries an on-chain attestation state

Every approved draft card must show an attestation block, and the block
must be honest about which of three states it is in: anchored on Monad
(tx link + doc hash), anchor failed (error + retry button), or not
configured (a muted line naming the env vars). The "Anchored on Monad"
count tile sits in the promotion counts grid. This flow works whether or
not a signer is configured — it verifies the UI is truthful in either
mode.

## The counts grid carries the anchored tile
Open /promotions.
Confirm the promotion counts grid contains a card titled "Anchored on
Monad" with a number, and a muted sub-line that states either that
approvals write a SHA-256 doc hash on-chain or that no signer is
configured.

## An approved draft shows its attestation state
If no draft card is visible, seed one first: go to /review and reject the
three Flop hands with hand codes KTo, K5s (pot 12), and K5s (pot 26) —
that auto-drafts a policy edit; then return to /promotions and approve it.
Find an "approved" draft card (status badge "approved").

Confirm the approved card contains:
- a "Download patch JSON" link and an "Attestation doc" link;
- an attestation block showing exactly one of:
  - an "ANCHORED" badge with a tx link whose href points at an explorer
    and a "sha256:" doc hash fragment, or
  - an "ANCHOR FAILED" badge with a "Retry anchor" button, or
  - a muted line saying on-chain anchoring is not configured.

## The attestation doc endpoint serves the verifiable preimage
Open /promotions/{draft-id}/attestation for the approved draft (the draft
id is the slug in the card header, e.g. "flop-bet-to-check").
Confirm the response is JSON with schema "sekats-attestation/v1", the
draft_id matching the card, a non-empty source_decisions array, and an
approved_at timestamp. The response headers carry X-Sekats-Doc-SHA256 —
that digest is what the contract stores under sha256("sekats/draft/" +
draft_id); anyone can re-hash the downloaded doc to check the anchor.
