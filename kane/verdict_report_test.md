---
mode: testing
url: http://localhost:8080/review
max_steps: 30
tags: [verdict-report, export, smoke]
---

# Verdicts persist and appear in the Verdict Report and JSON export

## Accept a hand on the review queue
Open the review queue page.
Find the first pending review item, note its hand code as reported_hand, and click its "Accept agent" button.

## The Verdict Report reflects the adjudication
Navigate to /verdicts.
Confirm the report contains a heading mentioning "accepted" together with reported_hand.
Confirm the summary table's Agent (accepted) column shows at least 1.

## The JSON export carries the same verdict
Navigate to /export?download=true.
Confirm the downloaded JSON contains an item whose hand_code is reported_hand with verdict "accepted".

## Verdicts survive a restart (persistence)
Stop the server process and start it again with the same DATA_DIR.
Open /verdicts again.
Confirm the accepted count is still at least 1 and reported_hand still appears — the verdict file on disk was reloaded.
