---
mode: testing
url: http://localhost:8080
max_steps: 30
tags: [live-bridge, smoke]
---

# Quality Surface shows where its numbers came from

## Open the Quality Surface page
Open the app's home page.

## Verify the data-source badge is present
Confirm the page shows a data-source badge (an element near the top labeled LIVE or FROZEN) that states where the numbers came from.
If the badge says LIVE: confirm it also shows a "latest decision" number in the form #12345.
If the badge says FROZEN: confirm it mentions a frozen export and points at the sync tool (text like "sync_live").

## Verify the broken-loop callout states its freshness
Confirm the broken-loop callout (about policy suggestions and how many were promoted) contains a line that says whether the numbers are current/live or stale/frozen.
