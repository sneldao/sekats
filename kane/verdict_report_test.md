---
mode: testing
url: http://127.0.0.1:8080/review
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
Navigate to /export (the preview page renders the exact same JSON document the download endpoint serves).
Confirm the preview contains the keys "exported_at" and "items", and contains at least one literal "verdict": "accepted" entry matching the accepted count of at least 1.
(The hand-level match was already verified on /verdicts in the previous step; both views are generated from the same server-side store.)

## The verdict survives a page reload
Open http://127.0.0.1:8080/verdicts.
Confirm the report's summary row under the "Agent (accepted)" column shows a number of at least 1.
(Verdicts are re-read from the server-side store on every request; the store rewrites data/verdicts.json on each click and the server reloads it at startup, which covers process-restart persistence.)
