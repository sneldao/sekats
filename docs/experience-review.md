# sekats — experience review & clarity pass

A review of the parlor (`/`, `/sheet`, `/check`, `/about`) across product design,
UI/UX, motion, styling, and performance — with a concrete language rewrite.

The short version: **the craft is already here; the story is not.** The site has
a distinctive look, a thematically perfect card metaphor, and real
perceived-performance work. What it lacks is a single, plain sentence a
first-time visitor can repeat, and a verdict moment that lands like it matters.
Everything below serves those two goals.

---

## 0. If you only do five things

1. **Make the verdict a moment.** The whole product is one bit — HOLD or DRIFT —
   and today it appears as quiet text on one card among five. Choreograph the
   reveal (stamp + page-wide color flood + timestamp). This is the memorable lever.
2. **Collapse the vocabulary.** The site currently speaks three glossaries at
   once: *parlor / canary / contract / cockpit / tape* (ops theater),
   *field / ledger / replay / posture* (poker), and
   *claim / pin / season / runner / process* (arena internals). Keep the poker
   metaphor, keep HOLD/DRIFT, and demote everything else to plain words.
   Concrete rewrites in §3.
3. **Earn trust with timestamps.** The memory cards show unmarked sample data;
   the probe returns `fetchedAt` but the UI never shows it. Stamp every memory
   card with "as of" and every verdict with "checked Ns ago · re-check".
   Trust is the product; right now the site asks for it on faith.
4. **Add an error path.** When the arena doesn't answer, the sheet shows a bare
   `ERROR` with no retry and no sentence. A trust tool that fails silently looks
   broken, not unlucky.
5. **Fix the small-text/contrast floor.** Critical instructions and labels are
   set in 8px letterspaced mono at ~2.6:1 contrast (`--ink-faint` on paper).
   Charming in the footer, hostile where the user must read to act.

---

## 1. What's already strong (keep, don't touch)

- **Art direction.** Warm bone paper, dither weave, Fraunces/Space Grotesk/Plex
  Mono. Nothing else in this space looks like this. The parlor conceit is
  coherent and genuinely memorable.
- **The card-deck metaphor on `/sheet`.** Five cards, one of them live, dealt in
  3D with a suit-coded ambient wash. It fits poker, it fits "a sheet," and the
  progressive disclosure (one card focused, neighbors peeking) is the right
  information hierarchy for this data.
- **Loading theater that respects time.** Pixel-grid + shimmer + *elapsed
  timer* during the probe is best-in-class perceived performance: it says
  "working," says what it's doing, and grounds the wait in a real clock.
- **Live vs memory tagging.** The `live` / `memory` tags on the sheet's guide
  line are the single most important UX idea on the site — an honest admission
  of which numbers are real. Build on it (§4.3).
- **Performance posture.** Zero frameworks, zero JS deps, inline CSS, ~40KB
  pages, `prefers-reduced-motion` honored everywhere, 44px touch targets,
  safe-area insets. This is a solid foundation to add motion *to*.
- **HOLD / DRIFT.** Short, poker-adjacent (hold'em), self-opposing, sticky.
  These two words are the brand. Keep them exactly.
- **The boot page's flip card** with idle float — the right handshake for
  "you're entering a card room, not a dashboard."

## 2. The core problem: one story, three vocabularies

Ask a first-time visitor what this site does and they'll struggle — not because
the design is confusing, but because **the same five concepts have three names
each**:

| The thing | What the site calls it |
|---|---|
| This website | parlor · cockpit · sheet · canary · sekats |
| The bot's claim | contract · claimed seasons · what the process believes · pins |
| The live truth | the arena · list-active · live field |
| The verdict | HOLD / DRIFT · canary state · season contract |
| The bot itself | the subject · the runner · the process · the player · stack_cartographer |

Every one of those words is defensible in isolation. Together they ask the
visitor to learn a glossary before they can read a sentence. The fix is not to
delete the personality — it's to pick **one name per concept** and let the
texture live in the styling instead of the nouns.

**The one-sentence story to hang everything on:**

> *stack_cartographer claims it's playing certain seasons.
> sekats checks that claim against the live arena — and calls it: HOLD or DRIFT.*

That sentence has three beats — **claim → check → verdict** — and every page,
tooltip, and label should be able to point at one of the three beats. When a
piece of copy can't, it's decoration and should be cut or moved to `/about`.

Recommended canonical glossary:

| Concept | Use | Retire |
|---|---|---|
| this site | **sekats** (wink: "stakes, reversed") | parlor, cockpit, canary-as-name |
| the bot | **stack_cartographer** / "the bot" | the subject, the runner, the process |
| the claim | **claims** / "claimed seasons" | contract (noun), pins, beliefs |
| the truth | **the live arena** / "live" | list-active, live field |
| the verdict | **HOLD / DRIFT** | — |
| the check page | **check any agent** | field scan, SCAN-as-verb |
| the memory cards | **memory · snapshot** | — (already good) |

And put the name wink on the site itself — it's currently only in the README,
so the name reads as a typo instead of a joke:

> *sekats — "stakes," reversed. We check the claims, not the cards.*

One line in the footer or `/about` makes the name memorable instead of puzzling.

---

## 3. Language pass — page by page

Principles: **question before answer** (lead with what the visitor is already
wondering, not the mechanism); **verbs over nouns** ("checks / claims / moved
on" beat "contract / canary / rotation"); **one name per concept** (§2); and
**don't burn the two good words** — everything around HOLD/DRIFT should get
simpler so they stand out.

### 3.1 `/` (landing)

| Where | Now | Proposed | Why |
|---|---|---|---|
| Punch | "Checks whether a poker agent's **claimed seasons** still match the live arena." | "**Is the poker bot still playing what it claims to be playing?** sekats checks the live arena and calls it." | Question-first; "claimed seasons" is jargon as the second noun a stranger meets. |
| Lede | "This is **not** the poker bot. stack_cartographer plays on DevFun Arena. This parlor only watches the public contract — no API key." | "This site is **not** the bot. **stack_cartographer** plays poker on DevFun Arena. sekats only reads what's public — no API key, no access." | "parlor" and "public contract" are the two least-clear words on the page. |
| Legend HOLD | "claimed seasons are still live" | "the claims check out" | Half the syllables; sets up DRIFT as the foil. |
| Legend DRIFT | "the arena moved on — the process is stale" | "the arena moved on — the claims are stale" | "The process" is operator-speak; the *claims* are what's stale. |
| CTA hint | "Public card · claimed vs arena right now" | "the live card — claims vs the arena, right now" | "Public card" means nothing before the visitor has met the deck. |
| Alt link | "memory sheet" | "how it plays (memory)" | Say what it is, not what it's called. |
| Footer | "not the player · watching the contract" | "not the player · watching the claims" | Glossary consistency. |

Keep the CTA **"See the live check"** as-is — it's good, and the Kane boot
test clicks exactly that string.

### 3.2 `/sheet`

The guide lines are already close; tighten and let the memory/live split do
more work:

| Where | Now | Proposed |
|---|---|---|
| Guide (public) | "live · claimed seasons vs the arena right now. HOLD = still true. DRIFT = seasons moved on." | "**live** · what the bot claims vs what the arena shows, right now. **HOLD** — claims check out. **DRIFT** — the arena moved on." |
| Guide (memory) | "memory · opponents this agent has faced. Snapshot — swipe back to Public for the live check." | "**memory** · opponents it has faced — a snapshot from its records, not the arena. The **Public** card is the live one." |
| Public tooltip | "The real canary: claimed seasons vs live arena list. HOLD or DRIFT." | "The live check: the bot's claimed seasons vs the arena's active list." |
| "claimed" tip | "Competition IDs this parlor (and the runner) still believe are current." | "Season ids the bot's operator says it's playing." |
| "live" tip | "What the arena says is active right now (list-active)." | "What the arena lists as active right now." |
| Seat, pre-probe | "awaiting probe" | "checking…" |
| Footer | "probe on load" | "checks on load" |

Two naming notes:

- The Public card's corner letter is **C** (for "contract"?) while the header
  says **Public** — a tiny incoherence on the most important card. Make the
  corner **P**. If you ever rename the visible label ("Live" reads clearest),
  keep `data-panel="public"` and the `#public` hash so Kane and links survive.
- *Field / Ledger / Replay / Posture* are atmospheric but abstract. The guide
  explains them, so renaming is optional — but **Opponents / Tally / Hands /
  Style** would let a stranger read the deck without the guide.


### 3.3 `/check`

This page has the hardest job: one input, three outcomes (seats scan,
disambiguation pick, HOLD/DRIFT verdict). The copy currently teaches all three
up front. Let the page teach one step at a time.

| Where | Now | Proposed |
|---|---|---|
| Punch | "Paste any public Arena agent. See where they sit — or pin claimed seasons for HOLD / DRIFT." | "Paste any public agent — a link, an id, or a name. See where they're seated right now." |
| Placeholder | "url · id · or a live name" | "agent link, id, or name" |
| Hint | "try stack_cartographer · see seats first · pin competition ids for HOLD / DRIFT" | "try stack_cartographer — or paste your own bot" |
| Scan nudge | "Where they sit on seasons that are still live. Pin the ids a runner still believes to get HOLD or DRIFT." | "Live seasons where this agent holds a rank. Run this bot? Add the season ids you claim to get a verdict." |
| Pin toggle | "pin claimed seasons" | "**I run this bot — check my claims**" |
| Pin note | "Match is HOLD. Miss is DRIFT — the season-rotation bug." | "Every claimed id still live → HOLD. Any id missing → DRIFT. Silent season rotation is the classic failure." |
| Pin placeholder | "competition ids the runner still pins" | "the season ids you claim — spaces or commas" |
| Pin button | "Watch HOLD / DRIFT" | "Check my claims →" |
| History label | "was" | "past seasons" |

The single biggest clarity win on this page is the pin toggle: "pin claimed
seasons" asks the visitor to already know what pinning is, what a claim is, and
why they'd bother. "**I run this bot — check my claims**" names the visitor's
situation instead of the mechanism.

### 3.4 `/about`

Already the clearest page — treat it as the reference voice for the others.

- "A small parlor for a public 'contract': competition IDs the process still
  believes, compared to what DevFun Arena lists as active." → "A public claim,
  checked: the seasons the bot's operator says it's playing, compared with
  what DevFun Arena lists as active."
- Add the name wink (§2) here even if it also lives in the footer.

### 3.5 Meta & manifest

Share text is the first sentence strangers read — it must survive alone:

- manifest / og description, now: "A parlor that watches whether a poker agent
  is still the agent it claims to be."
- proposed: "Is the poker bot still playing what it claims to be playing?
  sekats checks the live arena — HOLD or DRIFT, no API key."

---

## 4. Product & UX

### 4.1 The verdict deserves a reveal, not a text swap
Today `renderProbe()` swaps "pending" → "HOLD"/"DRIFT" with no transition —
the most important 200ms on the site is silent. Choreography in §5.1. The
verdict should also carry **when** it was true: the probe already returns
`fetchedAt`, but no page displays it. Show "checked 4s ago · re-check" under
the verdict. A live timestamp turns the verdict from an opinion into evidence,
and re-check gives a returning visitor something to do.

### 4.2 Failure is a state too
When the arena is unreachable, the sheet shows a bare `ERROR` and /check an
italic line. No retry, no sentence, no timestamp. For a trust product, silent
failure reads as *broken*, not *unlucky*. Add: a plain sentence ("the arena
didn't answer — its public API may be down"), the attempt time, and a **retry**
button. Client-side, cap the wait: the server caps curl at 20s; the page
should stop the shimmer gracefully past ~25s instead of counting forever.

### 4.3 The memory cards need provenance
Field / Ledger / Replay / Posture show specific numbers (2,798 hands; "Long
Tam · 89 fold"; "+0.26/hand") with no date or source. A stranger can't tell
these are a frozen snapshot, and a sophisticated visitor can't tell *when* it
was frozen — so the numbers quietly undermine the trust the Public card is
building. In order of effort:
1. Stamp each memory card "memory · as of \<date\>" (one shared constant).
2. Longer-term, generate the four cards from a real exported snapshot
   (`assets/memory.json`) so the sheet is honest by construction.

### 4.4 Landing hierarchy: promote `/check`
"check any agent" is the most useful, most shareable feature — the only thing
a visitor can do with *their own* context — and it's a tertiary text link.
Give it a quiet secondary button under the primary CTA; keep "how it plays
(memory)" and "about" as the tertiary pair.

### 4.5 After a scan, the path forward is muddy
`/check` resolves into seats — then what? The nudge tells operators to pin
ids, but the casual visitor (the common case) gets a dead end. Offer both
exits explicitly: "run this bot? check your claims" and "not yours? open their
live profile ↗". One screen, two exits.

### 4.6 Share the verdict
After any check, one-tap **"copy result"** (`HOLD · S14/S15 · #12 of 318 ·
checked just now · sekats.thisyearnofear.com`) turns every user into
distribution. Cheap, on-brand (a stamped slip), and it makes the verdict
portable to where operators actually talk.

### 4.7 Deck discoverability
Five cards, but on mobile the neighbors are thin slivers and the tick dots are
unlabeled — first-time visitors may never find cards 2–4. Cheap fixes:
- a one-time hint ("5 cards — swipe") that fades after first interaction;
- tick dots reveal their card's name on hover/focus (the `aria-label`s already
  exist — surface them visually);
- optionally fan the deck once on first load (Public → Field → Public, ~1.2s)
  to teach the gesture by showing it. Skip on `prefers-reduced-motion` and
  repeat visits (`localStorage`).

### 4.8 Small incoherences to sweep
- The sheet fetches `agent.quote` into `#agentQuote` but CSS hides it
  (`.identity .quote{display:none}`). "Ranges first, rivers last." is the most
  human sentence in the dataset — show it on the Public card once the verdict
  is the hero, or delete the dead code.
- Landing `<noscript>` styles reference `[data-chr]` / `.cursor`, elements
  that don't exist on that page — vestige of a removed effect. Clean up.
- v1–v3 dashboards are still served at guessable URLs with stale data. If
  they're an intentional trail, add `<meta name="robots" content="noindex">`
  so they don't compete with the canonical experience.


---

## 5. Motion design

The site already has good motion *manners* (one ease family, reduced-motion
support, no gratuitous loops). What's missing is **narrative motion** —
movement that tells the story of claim → check → verdict.

### 5.1 The verdict stamp (the signature moment)
On probe resolve, today: `textContent = "HOLD"`. Proposed, ~900ms total:

1. The pixel loader finishes its current cycle and fades (don't cut mid-blink).
2. The verdict **stamps in**: `scale(1.12) rotate(-1.2deg)` → `scale(1)
   rotate(0)`, ~350ms, overshoot ease (`cubic-bezier(.2,1.4,.3,1)`), like a
   rubber stamp hitting paper.
3. Simultaneously the ambient wash **floods to verdict color** — sage for
   HOLD, rose for DRIFT — at higher intensity than any channel wash (the .85s
   transition already exists; drive it from a `body[data-verdict]` attribute).
   On DRIFT the whole page should *feel* warmer; on HOLD, calmer.
4. The claimed-list rows resolve one by one (60ms stagger); the missing id, if
   any, strikes through in rose.
5. The footer stamp prints `HOLD · checked 12:03:44` with a tabular-nums
   tick-over.

All five are cheap transform/opacity moves on existing elements; all collapse
to an instant render under `prefers-reduced-motion`.

### 5.2 Deal-in transition (landing → sheet)
The landing's `leaving` fade and the sheet's `arrive` rise are two unrelated
clips. Make them one motion: on CTA click, the hero card settles (its float
stops, it squares to the viewer), the page fades *through* that moment, and on
/sheet the five cards **fan in** from center (50ms stagger, from
`translateX(0) rotateY(0)` to their dealt positions). It reads as: the card
you flipped becomes the deck you're dealt. Implementation: stagger the
existing `.card` transform transition via `transition-delay` on first load
only (a class on `<body>`, removed after); skip on hash-nav/reduced-motion.

### 5.3 Channel-change polish
- The `.wash` color transition (.85s) is lovely — keep.
- The guide line swaps via `innerHTML` with no transition; add a 160ms
  fade/slide so the narration changes as gracefully as the color.
- On tick tap, a 120ms scale bounce (`1 → .86 → 1`) on the dot — a haptic-like
  acknowledgment that costs nothing.
- Optional, desktop-only: the focused card tilts ±1.5deg toward the pointer
  (pointermove, lerped, `requestAnimationFrame`-throttled). Must stay under
  2deg or it fights the deck geometry.

### 5.4 Loading theater: tell the truth at the edges
The shimmer labels cycle on a timer, not real fetch stages — fine as theater;
the elapsed timer keeps it honest. One addition: past ~8s, append a status
line ("the arena is slow to answer — still trying"). Uncertainty with a clock
is tolerable; uncertainty with a stalled story feels broken.

### 5.5 What *not* to add
No sound, no confetti, no parallax-on-scroll, no infinitely looping
backgrounds. The parlor's calm is the brand; motion should spike at exactly
three moments — arrive, deal, verdict — and otherwise hold still.

---

## 6. Styling & visual system

### 6.1 Text size floor
The mono micro-label style (`font-size:.5rem` + `.2em` letter-spacing) is used
for the kicker, hints, card heads, list labels, and footer — nearly all
*instructional* text. At ~8px it fails as interface copy (fine as ornament).
Floor: **~11px (.68rem) minimum for anything the user must read to act**; keep
the 8px cut for the footer/ornamental layer only. This single change does more
for "intuitive" than anything else here.

### 6.2 Contrast
`--ink-faint` (#a89b8c on #f2eee5 ≈ 2.6:1) currently carries placeholders,
hints, tick dots, and the footer. Promote essential text to `--ink-soft`
(#6f6a5e ≈ 4.6:1); reserve `--ink-faint` for borders and ornament. DRIFT rose
(#c26a58 on paper ≈ 3.6:1) is fine at display sizes (the verdict) but small
"gone" rows need a weight bump or a ✕ glyph — which also fixes color-only
signaling.

### 6.3 Color semantics
Rose currently means: brand accent, link hover, Field suit, DRIFT, ERROR, and
"asked" highlight — too many jobs, and "brand" + "failure" are the worst two
to share. Proposal:
- **Verdict colors own their moments**: sage = HOLD/good, rose = DRIFT/error.
- Move brand/hover duty to ink + one warm accent (mustard is underused and
  distinct from both verdict colors); reassign the Field suit so no card suit
  collides with a verdict color.
- The verdict flood (§5.1) should be the only time the page takes a verdict
  color full-bleed, so it reads as *the answer*, not wallpaper.

### 6.4 One tokens file
The four pages duplicate the same `:root` block with drift (paper `#f2eee4`
vs `#f2eee5`; `--paper-2` `#ece5d6` vs `#ece5d8`; different accent subsets).
Extract one canonical token set (colors, type, easings, durations) and keep
page deltas explicit. Static duplication is a fine architecture for this
project — but make it a *choice*: a tiny include step, or a header comment
marking the shared block "edit in all four."

### 6.5 Type budget
Fraunces variable + Space Grotesk + Plex Mono is the right cast, but the
landing pulls Fraunces 300/400/500/600, Grotesk 300/400/500, Plex Mono
400/500/700. Audit actual usage (the design leans on Fraunces 300/400 italic +
600, Grotesk 300, Plex Mono 400/500) and drop the rest — each weight is
render-blocking bandwidth on the first impression. Keep `display=swap`.


---

## 7. Performance

The foundation is genuinely good (no framework, tiny pages, inline CSS). The
remaining costs, in order of user impact:

1. **Probe latency is the experience.** `/api/probe` has `Cache-Control:
   no-store` and re-fetches everything per call; name search scans up to 300
   leaderboard rows × every live play season. The watch theater covers ~2–4s
   gracefully, but repeat visitors pay it every time. Add a tiny server-side
   cache (30–60s TTL on `list-active` and name-search results; skip the cache
   when `claimed=` is present, since a verdict call should be fresh). A second
   load of `/sheet` should feel instant.
2. **Google Fonts are the only blocking third-party request.** Besides
   trimming weights (§6.5), consider self-hosting the three families (they're
   OFL) — removes two origins of handshake/DNS/TLS from every first visit and
   makes the local cockpit fully offline-capable.
3. **`og.jpg` is 339KB.** Recompress to ≤ ~120KB at 1200×630 q≈75 — it's the
   first thing every shared link fetches.
4. **Prefetch the next page.** `<link rel="prefetch" href="/sheet">` on the
   landing (after idle) makes the CTA transition effectively instant; same for
   `/check` on the sheet footer link. Pages are ~40KB, so this is nearly free.
5. **The dither layers** (`body::after` at `inset:-60vmax`, `.wvz` at
   `-40vmax`, both `mix-blend-mode:overlay`) force huge blend surfaces. Static,
   so the cost is mostly on scroll/repaint, but on low-end mobile GPUs an
   overlay blend over ~4× viewport area can jank the deck transition. Test on
   a mid-tier Android; if it's hot, bake the dot field into a small repeating
   data-URI at final opacity (no blend) and keep the radial mask.
6. **Client fetch guard.** Wrap the probe `fetch` in an `AbortController` with
   a ~25s cap so a hung connection resolves to the retry state (§4.2) instead
   of an infinite shimmer.

## 8. Accessibility

Mostly solid (aria-live on guide and watch regions, focus-visible tooltips,
44px targets, reduced-motion). The gaps:

- **Tabs aren't tabs.** `.ticks` has `role="tablist"` but the buttons lack
  `role="tab"` / `aria-selected`, and cards aren't `role="tabpanel"` with
  `aria-labelledby`. Screen-reader users get dots with no state. Wire it up;
  it's ten lines of attributes kept in sync in `select()`.
- **The verdict isn't announced.** Add `aria-live="polite"` (or reuse the
  watch region) on the verdict element so HOLD/DRIFT is spoken when it
  resolves — today only sighted users get the answer.
- **Keyboard switch, no focus shift.** Keys 1–5 change the card but leave
  focus and announcement nowhere. On tick activation, move focus to the
  selected card (it already has `tabindex="0"`).
- **Card focus ring.** Confirm a visible `:focus-visible` outline on `.card`;
  the current design language suggests it may be absent.
- **Color-only states.** "gone" rows and HOLD/DRIFT are text-distinguished —
  good — but add the ✕ glyph (§6.2) so the list scan doesn't rely on rose.

---

## 9. Priorities & constraints

**P0 — clarity & trust (copy + small code):**
glossary + page copy (§3), memory provenance stamps (§4.3), error/retry state
(§4.2), `fetchedAt` + re-check (§4.1), text-size floor (§6.1), verdict
aria-live (§8). Low risk, no visual redesign, transforms first-read
comprehension.

**P1 — the memorable layer (motion):**
verdict stamp + color flood (§5.1), deal-in fan (§5.2), guide crossfade +
tick bounce (§5.3), one-time deck hint (§4.7), promote /check (§4.4).

**P2 — system health:**
tokens consolidation (§6.4), color re-semantics (§6.3), probe cache + client
abort (§7.1/7.6), font weight audit + self-hosting (§6.5/§7.2), og.jpg
recompression (§7.3), tab wiring (§8), copy-result (§4.6).

**Kane test constraints** (from `tests/kane/**/_test.md`):
- `/` must keep a clickable **"See the live check"** leading to the sheet with
  **Public** selected.
- The sheet must keep the wordmark **"Stack Cartographer QC"**.
- The Public card must show a verdict plus visible **claimed** and **live**
  lists, and a link to the live arena profile.
Everything proposed above respects those strings/structures; any rename of
the visible "Public" label or CTA text needs the `_test.md` files updated in
the same commit.

