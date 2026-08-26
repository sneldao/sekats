---
mode: testing
url: http://127.0.0.1:8080/verdicts
max_steps: 20
tags: [verdicts, smoke]
---

# Verdicts report page renders human adjudications

## Open the Verdicts page
Open the verdicts report at /verdicts.

## Verify the report header is present
Confirm the page title or heading contains "Verdict Report".

## Verify it references the fixture data
Confirm the page contains at least one hand code from the fixture (e.g. "A7o" or "K5s") or mentions "shadow-LLM" or "agent was right".
