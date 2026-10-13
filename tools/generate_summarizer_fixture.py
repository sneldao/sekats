#!/usr/bin/env python3
"""Regenerate the explicitly synthetic summarizer fixture (no network calls)."""
import json
from datetime import datetime, timedelta, timezone
from pathlib import Path

BASE = datetime(2026, 10, 12, 9, tzinfo=timezone.utc)
decisions = []
for batch in range(12):
    project = f"Project Cedar-{batch + 1:02d}"
    completed, total = 18 + batch, 30 + batch
    cases = [
        (
            "unsupported_claim",
            f"{project} pilot: {completed} of {total} checks passed. "
            "The launch date remains undecided pending a security review.",
            f"{project} passed every check and will launch next week.",
            "publish",
            "The pilot update reads positively; a short definitive summary is easier to scan.",
            "Fixture evaluator found two unsupported claims: all checks passed and a confirmed launch date.",
            "revise",
            "Report the actual pass count and keep the launch date undecided. Do not invent certainty.",
        ),
        (
            "missing_attribution",
            f"In the {project} interview, a vendor estimates a 20% cost reduction. "
            "Finance has not validated that estimate.",
            f"{project} will reduce costs by 20%.",
            "publish",
            "The cost estimate is the most useful headline, so I omitted the speaker to save words.",
            "Fixture evaluator found the vendor estimate presented as an independently verified fact.",
            "revise",
            "Attribute the estimate to the vendor and retain Finance's validation caveat.",
        ),
        (
            "sensitive_record",
            f"{project} restricted HR note: employee SYNTHETIC-{batch + 1:03d} "
            "has a confidential accommodation request. Share only with authorized HR reviewers.",
            f"Employee SYNTHETIC-{batch + 1:03d} requested an accommodation on {project}.",
            "publish",
            "The request affects staffing, so I included it in the public project digest.",
            "Fixture evaluator blocked the simulated release because the audience lacks HR authorization.",
            "withhold",
            "Withhold the restricted HR record from the public digest; request authorized redaction review.",
        ),
        (
            "grounded_report",
            f"{project} build log: {completed} of {total} checks passed; "
            f"{total - completed} remain open. No release date has been set.",
            f"{project}: {completed}/{total} checks passed, {total - completed} open; no release date set.",
            "publish",
            "Every number and the release caveat are directly supported by the build log.",
            "Fixture evaluator verified all claims against the source and approved the simulated digest.",
            "publish",
            "The summary preserves both the exact counts and the release caveat.",
        ),
        (
            "unsupported_claim",
            f"{project} incident note: latency rose for 8 minutes. "
            "The cause is still under investigation; no data loss has been confirmed.",
            f"{project} outage was caused by a database failure and destroyed customer data.",
            "revise",
            "The proposed text asserts a cause and data loss the source does not establish; send it back for revision.",
            "Fixture evaluator accepted the revision request; the unsupported draft was not released.",
            "revise",
            "Revise to the observed latency duration and preserve the uncertainty about cause and data loss.",
        ),
    ]
    for case in cases:
        category, source, summary, action, rationale, outcome, shadow, shadow_why = case
        decision_id = len(decisions) + 1
        decisions.append(
            {
                "id": decision_id,
                "decided_at": int((BASE + timedelta(minutes=decision_id * 2)).timestamp() * 1000),
                "phase": "Editorial",
                "context_code": category,
                "inputs": {
                    "document_id": f"synthetic-doc-{decision_id:03d}",
                    "source_text": source,
                    "proposed_summary": summary,
                    "audience": "public project digest",
                },
                "action": action,
                "rationale": rationale,
                "outcome": outcome,
                "source": "mock-summarizer-v1",
                "accepted": 1,
                "review_action": shadow,
                "review_rationale": shadow_why,
            }
        )

suggestions = []
for index, summary in enumerate(
    [
        "Require source support for every quantitative claim.",
        "Preserve attribution and uncertainty when summarizing third-party estimates.",
        "Withhold restricted HR records from public digests.",
        "Retain unresolved incident cause and data-loss caveats.",
    ]
):
    suggestions.append(
        {
            "created_at": int((BASE + timedelta(minutes=30 * (index + 1))).timestamp() * 1000),
            "model": "mock-shadow-reviewer-v1",
            "summary": summary,
            "status": "pending",
            "error": None,
            "suggestion_json": json.dumps({"rule": summary}),
        }
    )

fixture = {
    "synthetic": True,
    "description": "60 offline, synthetic editorial decisions. No real HR records, LLM calls, or publications.",
    "decisions": decisions,
    "policy_suggestions": suggestions,
}
path = Path(__file__).resolve().parents[1] / "data" / "summarizer-decisions.json"
path.write_text(json.dumps(fixture, indent=2) + "\n")
print(f"Wrote {len(decisions)} synthetic decisions to {path}")
