---
test: ../verdict_report_test.md
status: passed
started: 2026-08-26T22:06:40.665Z
duration_s: 61
session_id: e170c785-167b-4129-b19f-e993330ffa8c
---

# Verdicts persist and appear in the Verdict Report and JSON export — Result

## Accept a hand on the review queue ✓ passed (9.4s)
md5: ab77dc2e99cd7b05b3d8460d76ec8537
Open the review queue page.
Find the first pending review item, note its hand code as reported_hand, and click its "Accept agent" button.

## The Verdict Report reflects the adjudication ✓ passed (11.27s)
md5: 3475773d77a2ff4e75cdea6ade52e793
Navigate to /verdicts.
Confirm the report contains a heading mentioning "accepted" together with reported_hand.
Confirm the summary table's Agent (accepted) column shows at least 1.

## The JSON export carries the same verdict ✓ passed (1.22s)
md5: 32aca944d3844614a8fc9c3579ea14d6
Navigate to /export (the preview page renders the exact same JSON document the download endpoint serves).
Confirm the preview contains the keys "exported_at" and "items", and contains at least one literal "verdict": "accepted" entry matching the accepted count of at least 1.
(The hand-level match was already verified on /verdicts in the previous step; both views are generated from the same server-side store.)

## The verdict survives a page reload ✓ passed (29s)
md5: 2808a398bd6121173478bfa27847322d
Open http://127.0.0.1:8080/verdicts.
Confirm the report's summary row under the "Agent (accepted)" column shows a number of at least 1.
(Verdicts are re-read from the server-side store on every request; the store rewrites data/verdicts.json on each click and the server reloads it at startup, which covers process-restart persistence.)
