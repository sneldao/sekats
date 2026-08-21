# assets/ — QC cockpit

The parlor UI is the operator console for the **public-contract canary**.
Plan: [`docs/public-contract.md`](../docs/public-contract.md). Serve it with
`python3 tools/qc_server.py` (not as `file://` — the Public channel needs the
live probe).

Palette: warm bone paper `#f2eee5`, warm ink `#1d1a16`, sparse rose `#c26a58`
/ mustard `#b58f3a`, plum `#7d5a73`, sage `#5c6b52`, teal `#3a6b65` on Public.

## Files

| File | What it is | Notes |
|------|-----------|-------|
| `contract.json` | Claimed public contract | Handle, profile URL, competition IDs the runner still pins. Diffed against live `list-active`. |
| `landing.html` | Device-boot **entry** (`/`) | Product name **sekats**. |
| `sheet.html` | **The sheet** (`/sheet`) | Five cards. Public is the canary. |
| `check.html` | **Check** (`/check`) | Paste any public agent. HOLD / DRIFT / SCAN. |
| `about.html` | **About** (`/about`) | What the parlor is, in the same voice. |
| `quality-dashboard-v4.html` | Redirect → `/sheet` | Old path. |
| `favicon.svg` `og.jpg` `robots.txt` `site.webmanifest` | Site chrome | Favicon, share image, crawlers. |
| `quality-dashboard.html` | **v1** — flat editorial poster | Evolution trail. |
| `quality-dashboard-v2.html` | **v2** — immersive everything-at-once | Evolution trail. |
| `quality-dashboard-v3.html` | **v3** — progressive disclosure | Evolution trail. |

## The journey (why v1–v3 exist)

1. **v1** proved the parlor palette but was a static "print page."
2. **v2** added immersion but dumped everything at once — verbose, hard to parse.
3. **v3** added the progressive-disclosure interaction model (the key win).
4. **v4** added the differentiated layering (dither / chroma / weave) on top of v3
   without reintroducing cards or clutter, then gained the **Public** channel.
5. **landing** framed it all — a device-boot page with a landscape flip card so the
   visitor knows what they're stepping into.

## Current direction

`/` → `/sheet` → `/check` → `/about`. Enter the operations opens the sheet.
`/check` is the any-agent canary; Kane still boots the parlor Public card.
Keys 1–5 (or swipe) select Field / Ledger / Replay / Posture / Public.
Public is the canary: claimed seasons vs live arena.
`v1`–`v3` are kept as the evolution trail.
