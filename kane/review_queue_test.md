---
mode: testing
url: http://localhost:8080/review
max_steps: 30
tags: [review-queue, smoke]
---

# Review Queue accept action updates state live

## Open the Review Queue page and read the starting counts
Open the review queue page.
Read the current Pending count and store it as pending_before.
Read the current Accepted count and store it as accepted_before.

## Accept the first pending review item
Find the first review item that shows a "pending" verdict badge, note its hand code as accepted_hand, and click its "Accept agent" button.

## Verify the verdict and counters updated without a full page reload
Confirm the review item whose hand code is accepted_hand now shows a verdict badge reading "accepted".
Confirm the Pending count now equals pending_before minus 1.
Confirm the Accepted count now equals accepted_before plus 1.
