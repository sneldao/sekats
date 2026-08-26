---
test: ../live_bridge_test.md
status: passed
started: 2026-08-26T20:38:19.080Z
duration_s: 108
session_id: c7de72d4-0093-4239-b0a9-25471815646e
---

# Quality Surface shows where its numbers came from — Result

## Open the Quality Surface page ✓ passed (11.6s)
md5: 9949ef39a784a6a875b2fe1bb058f6f5
Open the app's home page.

## Verify the data-source badge is present ✓ passed (63.4s)
md5: acbec71a0ad53e1fb97083bcdc2443fc
Confirm the page shows a data-source badge (an element near the top labeled LIVE or FROZEN) that states where the numbers came from.
If the badge says LIVE: confirm it also shows a "latest decision" number in the form #12345.
If the badge says FROZEN: confirm it mentions a frozen export and points at the sync tool (text like "sync_live").

## Verify the broken-loop callout states its freshness ✓ passed (27.7s)
md5: 981055613d10832bf60195d7409aecdb
Confirm the broken-loop callout (about policy suggestions and how many were promoted) contains a line that says whether the numbers are current/live or stale/frozen.
