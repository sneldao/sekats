---
workflow: product-launch-video
flow: automation
storyboard: yes
message: "Autonomous agents need a human governance layer — Sekats makes every approval and review a verifiable on-chain fact on Monad."
destination: hackathon-submission
aspect: 1920x1080
language: en
audience: "Monad Metropolis hackathon judges — Trust, Identity & AI Infrastructure track"
length: 90s
angle: "agents are already transacting with real money unsupervised; Sekats is the human review layer whose verdicts and policy approvals are anchored on Monad"
voice: elevenlabs
---

## Intent

A ~90-second demo video for the Monad Metropolis hackathon submission. Judges
watch this before opening the repo — it must land the thesis in 5 seconds, then
prove it with the *real* product, not slides. Dark, terminal-adjacent, precise;
Monad purple (#836EF9) accents on anything chain-touching. No fluff, no fake UI —
every console shot is the live fixture-mode console, every chain claim is a real
tx on monadscan.

## The story

1. Hook (0-8s): An autonomous poker agent on Monad has spent real MON. Who
   watches it? Its own shadow reviewer disagrees with it 15.7% of the time.
2. Evidence (8-25s): 6,243 shadow-LLM reviews, 84.3% agreement, only 6
   promotions ever. A quality console that turns policy into acceptance
   criteria.
3. The loop (25-50s): Reject repeated disagreements → console auto-drafts a
   policy edit → human approves → canonical attestation → hash anchored on
   Monad. Show the ANCHORED badge + monadscan.
4. Monad-native (50-75s): Permissionless jury — any visitor connects a wallet
   and sides with agent or shadow; real tx from their own wallet, live tally
   read back via eth_call. Then /anchors: the browser re-hashes the doc and
   reads the contract — "Match".
5. Close (75-90s): Two contracts, one boundary — gated SekatsAttestations for
   human policy approval (EIP-712), permissionless SekatsJury for public
   verdicts. "Governance for agents is public, inspectable, and composable."
   Live URL.

## Assets

- Live console: https://console.sekats.thisyearnofear.com (fixture mode)
- Screen recordings: footage/*.webm captured via agent-browser (real flows)
- monadscan pages: attester 0xE18EA17E7bEdf603a482013F59608446e95bB56c,
  anchor tx 0x3299f04af7c6d14a5ed4bef828498108b9ee4422cd56dc747d3652bc6337c072,
  SekatsAttestations v2 0x771a99F24E7Cd81d0937edcb797138AAC228FDfE,
  SekatsJury 0x0A8753aEE7684f8fB7773EfDD104B61C92fC9A14
- Voiceover: ElevenLabs via $ELEVENLABS_API_KEY (with-timestamps for captions)

## Customizations

- Jury vote and EIP-712 approval clips requiring the user's MetaMask are staged
  as placeholder shots — user records or approves those frames later.
- Captions from ElevenLabs word timestamps.

## Notes

- Never imply the public host holds a signer; jury txs are self-custodied by
  the visitor's wallet. Policy anchoring ≠ proof the agent executes policy —
  the contract itself says so; keep that honest framing, it's a strength.
- Eligibility honesty: console predates the window; the Monad layer (contracts,
  jury, EIP-712) is in-window work.
