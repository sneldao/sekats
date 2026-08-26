---
mode: testing
url: http://127.0.0.1:8080/trends
max_steps: 30
tags: [trends, smoke]
---

# Trends screen renders timeline and disagreement matrix

## Open the Trends page
Open the trends page at /trends.

## Verify the timeline table is present
Confirm the page contains a table with column headers "When", "Model", "Status", and "Summary".

## Verify the disagreement matrix is present
Confirm the page contains a table with column headers "Our action", "LLM action", "Count", "% of all", and at least one row where the "Our action" and "LLM action" differ (a disagreement row).

## Verify snapshot figures
Confirm the page shows a Linked Reviews count and an Agreement Rate percentage.
