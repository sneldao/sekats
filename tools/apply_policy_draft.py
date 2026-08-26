#!/usr/bin/env python3
"""Apply a sekats policy patch to an agent policy file.

A sekats promotion patch (downloaded from /promotions/{id}/patch after a
human approves a staged draft) says exactly what to append and where:

    {
      "schema": "sekats-policy-patch/v1",
      "target": "postflop.promotedFromHandReviews",
      "entry":  {"pattern": "...", "rule": "..."},
      ...
    }

This script is the only writer in the loop: it runs on whatever machine
holds the live policy file (the poker host, or a local copy of the poker
repo), it never talks to the console, and it refuses to run if the entry
is already present.

Usage:
    python3 tools/apply_policy_draft.py <patch.json> <policy.json>

The policy file is rewritten with 2-space indentation (JSON keys keep
their original order; only the new entry is added).
"""

import json
import sys
from pathlib import Path


def main() -> None:
    if len(sys.argv) != 3:
        print(__doc__.strip(), file=sys.stderr)
        sys.exit(2)

    patch_path, policy_path = Path(sys.argv[1]), Path(sys.argv[2])
    patch = json.loads(patch_path.read_text())
    if patch.get("schema") != "sekats-policy-patch/v1":
        sys.exit(f"unsupported patch schema: {patch.get('schema')!r}")

    target = patch["target"].split(".")
    entry = patch["entry"]

    policy = json.loads(policy_path.read_text())
    node = policy
    for part in target[:-1]:
        if not isinstance(node, dict) or part not in node:
            sys.exit(f"target section not found in policy: {'.'.join(target)}")
        node = node[part]
    if not isinstance(node, dict) or not isinstance(node.get(target[-1]), list):
        sys.exit(f"target is not a list: {'.'.join(target)}")
    arr = node[target[-1]]

    if any(
        isinstance(e, dict) and all(e.get(k) == v for k, v in entry.items())
        for e in arr
    ):
        print("entry already present; nothing to do")
        return

    arr.append(entry)
    policy_path.write_text(json.dumps(policy, indent=2) + "\n")
    print(
        f"appended 1 entry to {'.'.join(target)} "
        f"({len(arr)} entries total) via {patch.get('draft_id', '?')}"
    )


if __name__ == "__main__":
    main()
