---
mode: testing
url: http://localhost:8080/review
max_steps: 60
tags: [promotion-pipeline, closed-loop, smoke]
---

# Rejecting a repeated pattern auto-drafts a policy edit, and a human can approve it

The review queue lists the same 25 real hands in the same order in every
server session. Three of them are the same pattern — hand KTo on the Flop,
hand K5s on the Flop (pot 12), hand K5s on the Flop (pot 26) — all
"Agent: bet, Shadow-LLM: check". Rejecting all three crosses the default
threshold of 3, which auto-drafts a policy edit. Re-clicking an already
rejected item is a safe no-op, so this flow works on a fresh server and on
a server that already has the verdicts persisted.

## Reject the three flop bet-to-check hands
Open the review queue page.
Find the review item whose hand code is "KTo" and street is "Flop", and click its "Reject (LLM right)" button.
Find the review item whose hand code is "K5s", street is "Flop", and pot is 12, and click its "Reject (LLM right)" button.
Find the review item whose hand code is "K5s", street is "Flop", and pot is 26, and click its "Reject (LLM right)" button.

## Check for the auto-draft toast
Check whether a callout with the text "Policy edit drafted" is visible on the page.
If it is visible: confirm it mentions a pattern name and links to /promotions.
If it is not visible: continue to the next step (the draft was already staged in an earlier run — that is expected, verdicts persist across runs).

## The Promotion Pipeline shows a staged or approved draft with evidence
Navigate to /promotions.
Confirm at least one draft card is visible with a "Pattern" line (mentioning the Flop and the bet/check actions) and a "Drafted policy line".
Confirm that draft card's evidence table lists at least 3 decision rows.

## Approve the draft (if it is still pending)
If the draft card shows a "pending" status badge, click its "Approve edit" button.
Confirm the draft card now shows an "approved" status badge, a patch section with a "Download patch JSON" button, and no "Approve edit" button on that card.

## The patch JSON is valid and targets the policy section
Look at the patch JSON shown in the approved card's patch section.
Confirm the JSON has "schema" equal to "sekats-policy-patch/v1" and a "target" containing "promotedFromHandReviews".
Confirm the JSON has an "entry" object with "pattern" and "rule" fields, and a non-empty "source_decisions" array.
