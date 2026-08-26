---
test: ../trends_test.md
status: passed
started: 2026-08-26T20:56:13.493Z
duration_s: 94
session_id: fab3fef9-4bd0-40c8-8b12-a6c8816c83df
---

# Trends screen renders timeline and disagreement matrix — Result

## Open the Trends page ✓ passed (11.2s)
md5: 205d6a67194cb8cce75be690cfb0e2ac
Open the trends page at /trends.

## Verify the timeline table is present ✓ passed (18.6s)
md5: 3cdfd8becb1ed992e0b7ba3a4907fcd6
Confirm the page contains a table with column headers "When", "Model", "Status", and "Summary".

## Verify the disagreement matrix is present ✓ passed (39.7s)
md5: 1859ff6eb122139c621c73b617762aa6
Confirm the page contains a table with column headers "Our action", "LLM action", "Count", "% of all", and at least one row where the "Our action" and "LLM action" differ (a disagreement row).

## Verify snapshot figures ✓ passed (19.1s)
md5: 06fdfd12b88effef831a64e2c49ed80a
Confirm the page shows a Linked Reviews count and an Agreement Rate percentage.
