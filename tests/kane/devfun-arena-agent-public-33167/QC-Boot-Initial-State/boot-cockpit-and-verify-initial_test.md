---
mode: testing
max_steps: 30
timeout: 300
variables: {}
---

# Boot parlor and verify Public (live canary) is selected

## Step 1
Boot parlor and verify Public channel is selected

Verifies that navigating to the landing page and choosing the live check opens the sheet on the Public channel with the Stack Cartographer QC wordmark.

Navigate to http://127.0.0.1:8765/
Click **See the live check**
Verify the main title or wordmark on the dashboard includes Stack Cartographer QC.
Check that the Public channel is selected.
Observe the Public card content area for a contract verdict (HOLD, DRIFT, or pending).
