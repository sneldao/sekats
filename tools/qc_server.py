#!/usr/bin/env python3
"""Local QC cockpit: parlor UI + credential-free live arena probe.

Serves assets/ and GET /api/probe. Does not read .arena-credentials.
Kane and ./verify.sh should hit this process, not file://.

Default probe (no query) reads assets/contract.json — that is the Kane path.

Any-agent check (no API key):
  GET /api/probe?agent=<id or profile URL>
  GET /api/probe?agent=<id>&claimed=<id,id,...>
  python3 tools/qc_server.py --probe-only --agent <id>
  python3 tools/qc_server.py --probe-only --agent <id> --claimed id1,id2

Usage:
  python3 tools/qc_server.py
  python3 tools/qc_server.py --port 8765
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
from html import unescape
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse

ROOT = Path(__file__).resolve().parent.parent
REWRITE = {
    "/": "/landing.html",
    "/sheet": "/sheet.html",
    "/ops": "/sheet.html",
    "/about": "/about.html",
    "/check": "/check.html",
}
REDIRECT = {
    "/quality-dashboard-v4.html": "/sheet",
    "/landing.html": "/",
    "/sheet.html": "/sheet",
    "/about.html": "/about",
    "/check.html": "/check",
}
ASSETS = ROOT / "assets"
CONTRACT_PATH = ASSETS / "contract.json"
ARENA_ORIGIN = "https://arena.dev.fun"
ARENA_LIST_ACTIVE = ARENA_ORIGIN + "/api/arena/competition/list-active"
ARENA_AGENT_STATS = ARENA_ORIGIN + "/api/arena/agent/{agent_id}/stats"
ARENA_AGENT_PAGE = ARENA_ORIGIN + "/agent/{agent_id}"
ARENA_LEADERBOARD = ARENA_ORIGIN + "/api/arena/competition/leaderboard"
DEFAULT_PORT = 8765
CUID_RE = re.compile(r"^c[a-z0-9]{20,32}$", re.I)
TITLE_RE = re.compile(r"<title[^>]*>(.*?)</title>", re.I | re.S)
HANDLE_RE = re.compile(r">([a-z][a-z0-9]*(?:_[a-z0-9]+)+)<")


def load_claimed() -> dict:
    return json.loads(CONTRACT_PATH.read_text())


def split_ids(raw: str) -> list[str]:
    if not raw:
        return []
    return [part for part in re.split(r"[\s,;]+", raw.strip()) if part]


def parse_agent_ref(raw: str) -> str:
    """Accept a raw agent id or an arena.dev.fun profile URL. Never fetch the input URL."""
    text = (raw or "").strip()
    if not text:
        raise ValueError("missing agent id")
    if "://" in text or text.startswith("/") or "/agent/" in text:
        parsed = urlparse(text if "://" in text else "https://arena.dev.fun" + text)
        host = (parsed.hostname or "").lower()
        if host and host not in ("arena.dev.fun", "www.arena.dev.fun"):
            raise ValueError("agent URL must be on arena.dev.fun")
        parts = [p for p in parsed.path.split("/") if p]
        if "agent" not in parts:
            raise ValueError("agent URL must look like /agent/<id>")
        idx = parts.index("agent")
        if idx + 1 >= len(parts):
            raise ValueError("agent URL is missing an id")
        text = parts[idx + 1]
    text = text.strip("/")
    if not CUID_RE.match(text):
        raise ValueError("invalid agent id")
    return text


def classify_mode(name: str) -> str:
    blob = (name or "").lower()
    if "playground" in blob:
        return "playground"
    if "tournament" in blob:
        return "tournament"
    if "eval" in blob:
        return "eval"
    if "ladder" in blob or "heads-up" in blob or "heads up" in blob:
        return "ladder"
    return "other"


def match_score(query: str, name: str, handle: str) -> int | None:
    """0 exact, 1 prefix, 2 contains. None = no match."""
    q = (query or "").strip().lstrip("@").lower()
    if len(q) < 2:
        return None
    name_l = (name or "").lower()
    handle_l = (handle or "").lower()
    if handle_l == q or name_l == q:
        return 0
    if handle_l.startswith(q) or name_l.startswith(q):
        return 1
    if len(q) >= 4 and (q in handle_l or q in name_l):
        return 2
    return None


def curl_body(url: str, accept: str) -> str:
    """Public GETs. curl avoids macOS CPython SSL-cert gaps on urllib."""
    result = subprocess.run(
        ["curl", "-sS", "-m", "20", "-H", f"Accept: {accept}", url],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0:
        raise RuntimeError(result.stderr.strip() or f"curl exit {result.returncode}")
    return result.stdout


def curl_json(url: str) -> object:
    return json.loads(curl_body(url, "application/json"))


def fetch_list_active() -> list[dict]:
    data = curl_json(ARENA_LIST_ACTIVE)
    if not isinstance(data, list):
        raise RuntimeError("list-active did not return a JSON array")
    live = []
    for item in data:
        if not isinstance(item, dict):
            continue
        live.append(
            {
                "id": item.get("id") or item.get("competitionId") or "",
                "name": item.get("name") or "",
            }
        )
    return live


def fetch_seat(agent_id: str, competition_id: str) -> dict:
    url = (
        ARENA_AGENT_STATS.format(agent_id=agent_id) + f"?competitionId={competition_id}"
    )
    data = curl_json(url)
    if not isinstance(data, dict):
        return {"status": "ABSENT", "rank": None, "submissions": 0}
    rank = data.get("rank")
    submissions = int(data.get("totalSubmissions") or 0)
    joined = rank is not None or submissions > 0
    return {
        "status": "JOINED" if joined else "ABSENT",
        "rank": rank,
        "submissions": submissions,
        "bestRank": data.get("bestRank"),
        "score": data.get("totalScore"),
    }


def fetch_agent_stats(agent_id: str) -> dict:
    data = curl_json(ARENA_AGENT_STATS.format(agent_id=agent_id))
    return data if isinstance(data, dict) else {}


def fetch_leaderboard(competition_id: str, cap: int = 300) -> tuple[int, list]:
    rows: list = []
    offset = 0
    total = 0
    while offset < cap:
        url = (
            ARENA_LEADERBOARD
            + f"?competitionId={competition_id}&limit=100&offset={offset}"
        )
        data = curl_json(url)
        if not isinstance(data, dict):
            break
        total = int(data.get("total") or 0)
        chunk = data.get("data") or []
        if not isinstance(chunk, list) or not chunk:
            break
        rows.extend(chunk)
        offset += len(chunk)
        if total and offset >= total:
            break
    return total, rows


def fetch_field_size(competition_id: str) -> int | None:
    if not competition_id:
        return None
    try:
        url = ARENA_LEADERBOARD + f"?competitionId={competition_id}&limit=1"
        data = curl_json(url)
        if isinstance(data, dict) and data.get("total") is not None:
            return int(data["total"])
    except Exception:
        return None
    return None


def search_live_agents(query: str, live: list[dict]) -> list[dict]:
    play = [
        row
        for row in live
        if classify_mode(row.get("name") or "") in ("playground", "tournament")
    ]

    def hunt(row: dict) -> list[dict]:
        found = []
        try:
            total, board = fetch_leaderboard(row.get("id") or "")
        except Exception:
            return found
        for item in board:
            if not isinstance(item, dict):
                continue
            agent = item.get("agent") or {}
            if not isinstance(agent, dict):
                continue
            score = match_score(
                query, agent.get("name") or "", agent.get("handle") or ""
            )
            if score is None or not agent.get("id"):
                continue
            found.append(
                {
                    "id": agent.get("id"),
                    "name": agent.get("name") or "",
                    "handle": agent.get("handle") or "",
                    "rank": item.get("rank"),
                    "field": total,
                    "competitionId": row.get("id") or "",
                    "competitionName": row.get("name") or "",
                    "score": score,
                    "profileUrl": ARENA_AGENT_PAGE.format(agent_id=agent.get("id")),
                }
            )
        return found

    hits: list[dict] = []
    with ThreadPoolExecutor(max_workers=4) as pool:
        for batch in pool.map(hunt, play):
            hits.extend(batch)
    best_by_id: dict[str, dict] = {}
    for hit in hits:
        aid = hit["id"]
        prev = best_by_id.get(aid)
        if (
            prev is None
            or hit["score"] < prev["score"]
            or (
                hit["score"] == prev["score"]
                and (hit.get("rank") or 9999) < (prev.get("rank") or 9999)
            )
        ):
            best_by_id[aid] = hit
    ranked = list(best_by_id.values())
    ranked.sort(key=lambda h: (h["score"], h.get("rank") or 9999))
    return ranked


def resolve_agent(raw: str, live: list[dict]) -> dict:
    """URL / cuid → id. Otherwise search live playground + tournament names."""
    text = (raw or "").strip()
    if not text:
        raise ValueError("paste a profile URL, id, or live name")
    looks_direct = (
        "://" in text or "/agent/" in text or bool(CUID_RE.match(text.strip("/")))
    )
    if looks_direct:
        return {"id": parse_agent_ref(text), "matches": []}
    matches = search_live_agents(text, live)
    if not matches:
        raise ValueError('no live agent matching "%s"' % text.lstrip("@"))
    best = matches[0]["score"]
    top = [m for m in matches if m["score"] == best]
    return {"id": top[0]["id"] if len(top) == 1 else None, "matches": top}


def competitions_from_stats(
    stats: dict, live_ids: set[str]
) -> tuple[list[dict], list[dict]]:
    live_seats: list[dict] = []
    history: list[dict] = []
    for item in stats.get("competitions") or []:
        if not isinstance(item, dict):
            continue
        cid = item.get("arenaId") or ""
        name = item.get("arenaName") or ""
        rank = item.get("rank")
        row = {
            "id": cid,
            "name": name,
            "mode": classify_mode(name),
            "rank": rank,
            "status": "JOINED" if rank is not None else "ABSENT",
            "submissions": int(item.get("totalSubmissions") or 0),
            "score": item.get("totalScore"),
            "arenaStatus": item.get("arenaStatus") or "",
        }
        status = (item.get("arenaStatus") or "").lower()
        if cid in live_ids and rank is not None:
            live_seats.append(row)
        elif status == "ended" and rank is not None:
            history.append(row)
    return live_seats, history[:4]


def fetch_identity(agent_id: str) -> dict:
    profile = ARENA_AGENT_PAGE.format(agent_id=agent_id)
    html = curl_body(profile, "text/html")
    title = ""
    match = TITLE_RE.search(html or "")
    if match:
        title = unescape(re.sub(r"\s+", " ", match.group(1))).strip()
    name = (
        title.split(" entered", 1)[0].strip() if " entered" in title.lower() else title
    )
    handle = ""
    found = HANDLE_RE.search(html or "")
    if found:
        handle = found.group(1)
    return {
        "id": agent_id,
        "name": name,
        "handle": handle,
        "profileUrl": profile,
        "title": title,
    }


def rows_from_ids(ids: list[str], live: list[dict]) -> list[dict]:
    by_id = {row["id"]: row for row in live if row.get("id")}
    rows = []
    for cid in ids:
        live_row = by_id.get(cid) or {}
        name = live_row.get("name") or cid
        rows.append({"id": cid, "name": name, "mode": classify_mode(name)})
    return rows


def live_as_rows(live: list[dict]) -> list[dict]:
    rows = []
    for item in live:
        cid = item.get("id") or ""
        if not cid:
            continue
        name = item.get("name") or cid
        rows.append({"id": cid, "name": name, "mode": classify_mode(name)})
    return rows


def probe(*, agent_ref: str | None = None, claimed_raw: str | None = None) -> dict:
    claimed_file = load_claimed()
    file_agent = dict(claimed_file.get("agent") or {})
    file_rows = list(claimed_file.get("claimedCompetitions") or [])
    live = fetch_list_active()
    live_ids = {row["id"] for row in live if row.get("id")}

    matches: list[dict] = []
    if agent_ref:
        resolved = resolve_agent(agent_ref, live)
        if resolved["id"] is None:
            return {
                "ok": True,
                "kind": "pick",
                "fetchedAt": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
                "verdict": "PICK",
                "query": agent_ref,
                "matches": resolved.get("matches") or [],
                "agent": {},
                "claimed": [],
                "live": live,
                "missing": [],
                "seats": [],
                "history": [],
                "invariants": claimed_file.get("invariants") or [],
            }
        agent_id = resolved["id"]
        matches = resolved.get("matches") or []
    else:
        agent_id = file_agent.get("id") or ""

    if claimed_raw is not None:
        ids = split_ids(claimed_raw)
        if ids:
            kind = "contract"
            rows = rows_from_ids(ids, live)
        else:
            kind = "scan"
            rows = []
    elif agent_ref:
        kind = "scan"
        rows = []
    else:
        kind = "contract"
        rows = file_rows

    stats: dict = {}
    if agent_ref and agent_id:
        try:
            stats = fetch_agent_stats(agent_id)
        except Exception:
            stats = {}

    if kind == "scan":
        seats, history = competitions_from_stats(stats, live_ids)
        with ThreadPoolExecutor(max_workers=4) as pool:
            sizes = list(pool.map(lambda s: fetch_field_size(s.get("id") or ""), seats))
        for seat, size in zip(seats, sizes):
            if size is not None:
                seat["field"] = size
        missing = []
        verdict = "SCAN"
        claimed_out: list[dict] = []
    else:
        history = []
        seat_source = rows
        missing = [
            {
                "id": row.get("id") or "",
                "name": row.get("name") or "",
                "mode": row.get("mode"),
            }
            for row in rows
            if (row.get("id") or "") not in live_ids
        ]
        verdict = "DRIFT" if missing else "HOLD"
        claimed_out = rows

        def seat_for(row: dict) -> dict:
            cid = row.get("id") or ""
            seat = {
                "mode": row.get("mode") or classify_mode(row.get("name") or ""),
                "id": cid,
                "name": row.get("name") or "",
                "status": "ABSENT",
            }
            if agent_id and cid:
                try:
                    seat.update(fetch_seat(agent_id, cid))
                except Exception as exc:
                    seat["status"] = "ERROR"
                    seat["error"] = str(exc)
            return seat

        with ThreadPoolExecutor(max_workers=4) as pool:
            seats = list(pool.map(seat_for, seat_source))

    if agent_ref:
        agent = {
            "id": agent_id,
            "handle": stats.get("handle") or "",
            "name": stats.get("name") or "",
            "quote": stats.get("quote") or "",
            "profileUrl": ARENA_AGENT_PAGE.format(agent_id=agent_id),
        }
        if not agent["name"] and not agent["handle"]:
            try:
                ident = fetch_identity(agent_id)
                agent["name"] = ident.get("name") or ""
                agent["handle"] = ident.get("handle") or ""
            except Exception:
                pass
        if agent_id == file_agent.get("id"):
            agent["quote"] = agent["quote"] or file_agent.get("quote") or ""
            agent["handle"] = agent["handle"] or file_agent.get("handle") or ""
            agent["name"] = agent["name"] or file_agent.get("name") or ""
    else:
        agent = file_agent

    joined = [s for s in seats if s.get("status") == "JOINED"]
    play_modes = [s for s in seats if s.get("mode") in ("tournament", "playground")]
    play_joined = [s for s in play_modes if s.get("status") == "JOINED"]
    return {
        "ok": True,
        "kind": kind,
        "fetchedAt": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "verdict": verdict,
        "seatVerdict": "SEATED" if play_joined else "ABSENT",
        "joinedCount": len(joined),
        "agent": agent,
        "claimed": claimed_out,
        "live": live,
        "missing": missing,
        "seats": seats,
        "history": history,
        "matches": matches,
        "invariants": claimed_file.get("invariants") or [],
    }


class Handler(SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=str(ASSETS), **kwargs)

    def log_message(self, fmt: str, *args) -> None:
        sys.stderr.write("%s - %s\n" % (self.address_string(), fmt % args))

    def do_GET(self) -> None:
        parsed = urlparse(self.path)
        path = parsed.path
        if path != "/" and path.endswith("/"):
            path = path.rstrip("/")
        if path in ("/api/probe", "/api/probe.json"):
            self._send_probe(parsed.query)
            return
        if path == "/sitemap.xml":
            self._send_sitemap()
            return
        if path in REDIRECT:
            self.send_response(302)
            self.send_header("Location", REDIRECT[path])
            self.send_header("Cache-Control", "no-store")
            self.end_headers()
            return
        if path in REWRITE:
            self.path = REWRITE[path]
        elif "." not in path.rsplit("/", 1)[-1]:
            candidate = path + ".html"
            if (ASSETS / candidate.lstrip("/")).is_file():
                self.path = candidate
        super().do_GET()

    def _send_sitemap(self) -> None:
        host = self.headers.get("Host") or "127.0.0.1:8765"
        base = "http://%s" % host
        urls = ["/", "/sheet", "/check", "/about"]
        body = (
            '<?xml version="1.0" encoding="UTF-8"?>\n'
            '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n'
            + "".join("  <url><loc>%s%s</loc></url>\n" % (base, u) for u in urls)
            + "</urlset>\n"
        ).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/xml; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _send_probe(self, query: str) -> None:
        qs = parse_qs(query, keep_blank_values=True)
        agent_ref = (qs.get("agent") or [None])[0]
        claimed_raw = qs["claimed"][0] if "claimed" in qs else None
        try:
            payload = probe(
                agent_ref=agent_ref or None,
                claimed_raw=claimed_raw,
            )
            body = json.dumps(payload).encode("utf-8")
            self.send_response(200)
        except ValueError as exc:
            body = json.dumps(
                {"ok": False, "error": str(exc), "verdict": "ERROR"}
            ).encode("utf-8")
            self.send_response(400)
        except Exception as exc:
            body = json.dumps(
                {"ok": False, "error": str(exc), "verdict": "ERROR"}
            ).encode("utf-8")
            self.send_response(502)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def main() -> int:
    parser = argparse.ArgumentParser(
        description="QC cockpit + live public-contract probe"
    )
    parser.add_argument("--port", type=int, default=DEFAULT_PORT)
    parser.add_argument(
        "--probe-only", action="store_true", help="print probe JSON and exit"
    )
    parser.add_argument(
        "--agent", help="agent id, arena.dev.fun profile URL, or live name"
    )
    parser.add_argument(
        "--claimed",
        help="comma-separated competition ids the runner still pins (omit for a field scan)",
    )
    args = parser.parse_args()
    if args.probe_only:
        claimed_raw = args.claimed if args.claimed is not None else None
        print(
            json.dumps(probe(agent_ref=args.agent, claimed_raw=claimed_raw), indent=2)
        )
        return 0
    server = ThreadingHTTPServer(("127.0.0.1", args.port), Handler)
    print(f"QC cockpit  http://127.0.0.1:{args.port}/", flush=True)
    print(f"sheet       http://127.0.0.1:{args.port}/sheet", flush=True)
    print(f"check       http://127.0.0.1:{args.port}/check", flush=True)
    print(f"about       http://127.0.0.1:{args.port}/about", flush=True)
    print(f"probe       http://127.0.0.1:{args.port}/api/probe", flush=True)
    print("Ctrl-C to stop.", flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nstopped")
        return 0
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
