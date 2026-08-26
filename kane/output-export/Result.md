---
test: ../export_test.md
status: passed
started: 2026-08-26T20:58:01.082Z
duration_s: 93
session_id: 60522eeb-6ac3-43b5-b5d7-681689064e6c
---

# Export endpoint returns valid verdict JSON — Result

## Open the export preview page ✓ passed (10.1s)
md5: 0ae4c074e038748fe5180efb7921d479
Open the export page at /export.

## Verify it shows a JSON preview with items ✓ passed (30s)
md5: 3be9bee61b1991158c80a625d93037dd
Confirm the page contains the word "exported_at" and at least one review item field (e.g. "hand_code").

## Verify the download endpoint returns valid JSON ✓ passed (46.2s)
md5: 803be135c1c07cab28fa28b7e777bd96
Open http://127.0.0.1:8080/export?download=true and confirm the response is valid JSON containing an "items" array with at least one entry that has a "decision_id" field.
