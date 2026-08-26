#!/usr/bin/env bash
# Sync a WAL-consistent snapshot of the agent's live SQLite decision log
# into data/live/, where the sekats server opens it read-only.
#
# Why this shape:
#   - The DB runs on the poker host and is written in WAL mode by the live
#     process. Copying the raw .sqlite3 file with rsync can miss un-
#     checkpointed WAL frames; SQLite's online-backup API (python3's
#     sqlite3.backup) produces a consistent snapshot regardless.
#   - No credentials or host access are stored in this repo: the script
#     uses whatever SSH alias the operator has configured.
#
# Usage:
#   tools/sync_live.sh
#   SEKATS_DB_HOST=other-host SEKATS_DB_REMOTE=/path/to/db.sqlite3 tools/sync_live.sh
#
# After a sync, the running server picks the new file up on the next page
# request (it re-checks the snapshot's mtime); no restart needed.

set -euo pipefail

HOST="${SEKATS_DB_HOST:-nuncio-vultr}"
REMOTE_DB="${SEKATS_DB_REMOTE:-/home/linuxuser/arena-poker/arena-poker.sqlite3}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST="$ROOT/data/live/arena-poker.sqlite3"
TMP="$ROOT/data/live/.incoming.sqlite3"

mkdir -p "$ROOT/data/live"

echo "snapshotting $REMOTE_DB on $HOST ..."
ssh "$HOST" "SEKATS_DB_REMOTE='$REMOTE_DB' python3 -c '
import os, sqlite3
src = sqlite3.connect(\"file://\" + os.environ[\"SEKATS_DB_REMOTE\"] + \"?mode=ro\", uri=True)
dst = sqlite3.connect(\"/tmp/sekats-live-snapshot.sqlite3\")
src.backup(dst)
dst.close()
src.close()
print(\"  backup complete\")
'"

echo "copying snapshot ..."
scp -q "$HOST":/tmp/sekats-live-snapshot.sqlite3 "$TMP"

# Atomic swap: the server re-checks the destination's mtime on each
# request; a half-copied file must never be the visible snapshot.
mv "$TMP" "$DEST"

echo "synced: $DEST ($(du -h "$DEST" | cut -f1))"
echo "the running server will pick it up on the next page load."
