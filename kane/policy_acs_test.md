---
mode: testing
url: http://localhost:8080/policy
max_steps: 30
tags: [policy-acs, smoke]
---

# Policy-as-ACs flags a real violation

## Open the Policy-as-ACs page
Open the policy page.

## Verify rules ran and a violation was caught
Confirm the page shows how many decisions were checked and lists the rules that were run.
Confirm the Violations Found count is exactly 1, and a violations table is visible with at least one row.
