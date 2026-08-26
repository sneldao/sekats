---
mode: testing
url: http://127.0.0.1:8080/export
max_steps: 20
tags: [export, smoke]
---

# Export endpoint returns valid verdict JSON

## Open the export preview page
Open the export page at /export.

## Verify it shows a JSON preview with items
Confirm the page contains the word "exported_at" and at least one review item field (e.g. "hand_code").

## Verify the download endpoint returns valid JSON
Open http://127.0.0.1:8080/export?download=true and confirm the response is valid JSON containing an "items" array with at least one entry that has a "decision_id" field.
