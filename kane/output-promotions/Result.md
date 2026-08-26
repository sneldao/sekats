---
test: ../promotions_test.md
status: passed
started: 2026-08-26T20:40:24.092Z
duration_s: 284
session_id: d5e734bd-b7c5-4c65-81e5-8dbb795b6975
---

# Rejecting a repeated pattern auto-drafts a policy edit, and a human can approve it — Result

## Reject the three flop bet-to-check hands ✓ passed (100.8s)
md5: 31beee93338c27ee4547d5d3c11181d6
Open the review queue page.
Find the review item whose hand code is "KTo" and street is "Flop", and click its "Reject (LLM right)" button.
Find the review item whose hand code is "K5s", street is "Flop", and pot is 12, and click its "Reject (LLM right)" button.
Find the review item whose hand code is "K5s", street is "Flop", and pot is 26, and click its "Reject (LLM right)" button.

## Check for the auto-draft toast ✓ passed (43.7s)
md5: 9fb7884e5bfd23491771e23b06c19a2f
Check whether a callout with the text "Policy edit drafted" is visible on the page.
If it is visible: confirm it mentions a pattern name and links to /promotions.
If it is not visible: continue to the next step (the draft was already staged in an earlier run — that is expected, verdicts persist across runs).

## The Promotion Pipeline shows a staged or approved draft with evidence ✓ passed (36.2s)
md5: b0913d965b7f7ac08d93513934c3d8d8
Navigate to /promotions.
Confirm at least one draft card is visible with a "Pattern" line (mentioning the Flop and the bet/check actions) and a "Drafted policy line".
Confirm that draft card's evidence table lists at least 3 decision rows.

## Approve the draft (if it is still pending) ✓ passed (64.4s)
md5: 97b235cf1742ac13a799992c4eb183b0
If the draft card shows a "pending" status badge, click its "Approve edit" button.
Confirm the draft card now shows an "approved" status badge, a patch section with a "Download patch JSON" button, and no "Approve edit" button on that card.

## The patch JSON is valid and targets the policy section ✓ passed (33.1s)
md5: b020ff52775160f71b8bc5c1ad9f903f
Look at the patch JSON shown in the approved card's patch section.
Confirm the JSON has "schema" equal to "sekats-policy-patch/v1" and a "target" containing "promotedFromHandReviews".
Confirm the JSON has an "entry" object with "pattern" and "rule" fields, and a non-empty "source_decisions" array.
