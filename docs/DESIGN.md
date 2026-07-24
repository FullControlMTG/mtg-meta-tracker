# MTG Meta Tracker — Design

Meta-analysis tool for a local MTG cube playgroup. Users upload cube decklists
(optionally with a win/loss record), and the app aggregates them into an
analytics layer that surfaces what gets picked, what performs, and what plays
well together.

---

## 1. Architecture

```
                    ┌──────────────────────────────────────────┐
                    │                Next.js (App Router)        │
  browser  ◀──────▶ │  • /decks, /cubes, /analytics  dynamic     │
                    │  • /decks/[id], /users/[name]  ISR         │
                    │  • /analytics/[cube]           ISR, charts │
                    │  • /decks/new       live color inference    │
                    └───────────────┬──────────────┬────────────┘
                        rewrites /api│              │POST /api/revalidate
                                     ▼              ▲ (on-demand render)
                    ┌──────────────────────────────┴────────────┐
                    │              Go backend (chi)               │
                    │  httpapi ─ appctx (Caller) ─ auth           │
                    │  store (pgx) ── PostgreSQL                  │
                    │  scryfall client   ingest (pasted lists)    │
                    │  jobs worker ── analytics engine            │
                    └───────────────┬────────────────────────────┘
                                    ▼
                              Scryfall API
```

- **Frontend / backend split.** Next.js owns rendering + UX. Go owns data,
  auth, external integrations, and analytics. They talk JSON over `/api`, proxied
  by a Next.js rewrite so the session cookie stays same-site (no CORS headaches).
- **Render-on-update.** When a deck, record, or the cube pool changes, the Go
  backend (after recompute) calls a Next.js `/api/revalidate` route handler with
  a shared secret, which runs `revalidatePath` for the affected cached pages.
  Detail pages therefore render once and only re-render on real change. Rendering
  is chosen **per page**: the index pages (`/`, `/decks`, `/cubes`, `/analytics`)
  are `force-dynamic` — they are cheap, and a stale list is more confusing than a
  fresh query is expensive — while detail pages are ISR (`revalidate = 3600` for
  decks/users/cube analytics, `300` for cards and cube pools).
- **Trigger-driven analytics.** Any write that affects aggregates enqueues a job;
  a worker coalesces triggers and recomputes snapshots.

### Repo layout

```
backend/
  cmd/server/            main.go — wiring
  internal/
    appctx/              Caller context (public vs user vs admin)
    config/              env config
    domain/              core types: color bitset, enums
    store/               pgx repositories
      schema.sql         full schema — embedded, applied on every startup
    auth/                password (argon2id), sessions, middleware
    httpapi/             chi router + handlers  ← source of truth for routes
    scryfall/            batched card client (exact printings)
    ingest/              cube syncer: pasted list → resolve → diff pool
    decklist/            list parser + card resolver
    images/              self-hosted card-image cache
    moxfield/            publicId URL parsing (display metadata only)
    analytics/           recompute engine + queries
    jobs/                queue + worker + scheduler
    revalidate/          Next.js revalidate webhook client
db/                      scratch dir for gitignored dumps — NOT the schema
frontend/                Next.js app (app/, components/, lib/)
docs/                    this file + ROADMAP.md
```

**The schema is not a migration set.** `backend/internal/store/schema.sql` is
embedded in the binary and re-applied by `store.EnsureSchema` on every boot, so
it must stay idempotent — a statement that fails against a populated database
stops the server from starting. New columns go in the *Idempotent migrations*
section at the bottom of the file, not in the `CREATE TABLE` block (which is a
no-op once the table exists).

---

## 2. The request Context (Caller)

Every backend operation receives a `context.Context` carrying a `Caller`, built
by auth middleware from the session cookie (or absent, for public calls).

```go
type CallerKind int
const ( Public CallerKind = iota; Authenticated )

type Role int
const ( RoleUser Role = iota; RoleAdmin )

type Caller struct {
    Kind   CallerKind
    UserID uuid.UUID   // zero when Public
    Role   Role
}
```

- **Public context** — anonymous read access (browse decklists, users, analytics).
- **User context** — carries the authenticated user id + role.

Authorization lives in small predicates the handlers/store consult, e.g.
`caller.CanMutateDeck(deck)` = `Role==Admin || deck.UserID==caller.UserID`.
Read endpoints accept `Public`; create/update/delete require `Authenticated` and
an ownership/role check. This keeps a single, auditable place per rule.

---

## 3. Data model

Color identity is a 5-bit **bitset** (`SMALLINT`, 0–31):
`W=1, U=2, B=4, R=8, G=16`; colorless = 0.

### Core tables

| table | purpose | key columns |
|---|---|---|
| `users` | accounts | `id, username, email, display_name, bio, avatar_url, role, password_hash (nullable)` |
| `oauth_accounts` | Google links (table only — OAuth is unimplemented) | `user_id, provider, provider_account_id` |
| `sessions` | server-side sessions (revocable) | `id (token), user_id, expires_at` |
| `cubes` | a card pool = one pasted list | `id, name, card_list (raw paste, source of truth), content_hash, moxfield_public_id (display only), last_synced_at` |
| `cards` | Scryfall cache | `scryfall_id (pk), oracle_id, name, slug (generated), cmc, type_line, colors, color_identity, rarity, image_*, raw jsonb` |
| `cube_cards` | pool membership + history | `cube_id, card_id, added_at, removed_at (nullable), is_active` |
| `cube_sync_progress` | live progress for the admin sync UI | `cube_id (pk), status, cards_total, images_total/done/failed, unresolved text[]` |
| `decklists` | deck + metadata + record | see below |
| `decklist_cards` | normalized deck contents | `decklist_id, card_id, card_name, quantity, is_resolved, board` |

`cards.slug` is a **generated** column (`STORED`) powering `/cards/<slug>`, so it
can never drift from the name. It is not unique — two printings of a name are two
rows — so slug lookups tie-break rather than assume one hit.

`cube_sync_progress.unresolved` holds the names Scryfall could not resolve on the
last sync. They are dropped from the pool, so surfacing them is what keeps a typo
in the pasted list from silently shrinking the cube.

### `decklists`

Metadata + the list + the record all live together (record is nullable and
added after the fact), per requirement.

```
id                uuid pk
cube_id           fk
user_id           fk           -- uploader; owner + admins get U/D
name              text
description       text null
color_identity    smallint     -- inferred bitset (splashes excluded — see §8)
splash_colors     smallint     -- inferred bitset of sub-threshold colors
archetype         text null    -- CHECK enum: aggro | control | midrange | tempo | combo
source_url        text null    -- external deck link
decklist_raw      text         -- raw "1 Lightning Bolt\n…" (fits varchar/text)
card_count        int
status            text         -- draft | active | archived
-- record (nullable, added later; updating these triggers recompute)
games_played      int  default 0
wins              int  default 0
losses            int  default 0
event_name        text null
played_at         date null
record_updated_at timestamptz null
winrate           numeric GENERATED ALWAYS AS
                    (CASE WHEN games_played>0 THEN wins::numeric/games_played END) STORED
created_at, updated_at
```

`decklist_cards` is the backbone of card-level analytics — one row per card, with
`card_id` resolved against the cube pool (`is_resolved=false` when a name can't be
matched, so we can flag import problems without losing data).

---

## 4. Analytics schema (the core of this tool)

Analytics are **precomputed snapshots** written by the recompute engine and read
cheaply by the (mostly static) pages. Each recompute is a `run`; keeping runs
gives us history / time-series for free.

### Bookkeeping

```
analytics_runs(
  id, cube_id, trigger, status,
  decks_included, games_included,
  started_at, finished_at, is_current bool)
```
`is_current` marks the latest good run per cube; pages read `WHERE is_current`.

### 4.1 Color stats — `color_stats`

Four facets in one table so the dashboard can slice color performance several ways:

```
color_stats(run_id, facet, facet_key,
            deck_count, games, wins, losses,
            winrate)
```
- `facet = 'exact_identity'` → `facet_key` = the 0–31 bitset (WUBRG combos).
- `facet = 'single_color'`  → `facet_key` = one color bit (decks *containing* W…).
- `facet = 'color_count'`   → `facet_key` = 0–5 (mono/two/three-color…).
- `facet = 'splash_color'`  → `facet_key` = one color bit (decks *splashing* W…).

Answers "do blue decks win more?", "is two-color better than five-color?", and
per-combo winrates. The `splash_color` facet is the **only** place splashes are
counted — they are excluded from the other three, so a GW deck with two red cards
does not read as a three-color deck (§8).

### 4.2 Card stats — `card_stats`

Per pool card, the "picked most" + "performs best" signals from the README:

```
card_stats(run_id, card_id,
           deck_count,            -- how many decks run it
           inclusion_rate,        -- deck_count / total_decks  → popularity
           games, wins, losses,
           winrate)               -- winrate of the decks running it
```
- **Popularity** = `inclusion_rate`. This is the table's sort order.
- **Performance** = `winrate`, read alongside `deck_count`/`games` for context.

Note what `winrate` is *not*: a card's games are its **decks'** games, attributed
wholesale — every card in a deck inherits that deck's full record. See §4.5.

### 4.3 Co-occurrence / "played with" — `card_pair_stats`

Powers the "most often played with" list on a card page:

```
card_pair_stats(run_id, card_a_id, card_b_id,
                co_count,            -- decks with both
                pair_winrate)        -- winrate of the decks playing both
```
Stored only for pairs with `co_count >= 2` to bound the n² blow-up, both
directions per pair, ranked by `co_count` descending. Because the ranking is by
raw co-occurrence, the list skews toward cube staples — it answers "what usually
shares a deck with this card", not "what is *specifically* associated with it".

### 4.4 Meta overview + deck-property correlations

```
meta_snapshot(run_id, total_decks, total_games, overall_winrate,
              avg_cmc, avg_color_count, mono_share, multi_share, …)

deck_metric_stats(run_id, metric, bucket,       -- e.g. metric='avg_cmc'
                  deck_count, winrate)          -- winrate by CMC/creature-count bucket
```
Lets the dashboard chart "does a lower curve win?" and headline meta numbers.

### 4.5 Derived statistics: what we removed, and why

The engine used to compute a Bayesian-shrunk winrate, its **lift** over the
global mean, a **Wilson** lower bound, and association-rule **support/confidence/
lift** for pairs. All of it has been removed. The reasoning is worth keeping, so
nobody reintroduces it by reflex:

- **The input doesn't support it.** We record a *per-deck* aggregate (games, wins,
  losses) — there is no per-game or per-card data. So a card's "games" are its
  deck's games, attributed wholesale, and every card in a deck shares one number.
  Shrinkage and Wilson bounds are the right tools for a noisy *sample of a card's
  own outcomes*; here they were applying rigorous machinery to an attribution that
  is itself the weak link. They dressed one number up as three.
- **It was unexplained jargon.** "Lift" and "Wilson" appeared as bare column
  headers. A stat a reader can't interpret isn't a feature.

What's shown now is what the data actually says: popularity (`inclusion_rate`),
the record (`deck_count`, `games`, `winrate`), and raw co-occurrence (`co_count`,
`pair_winrate`) — each with a hover (i) explaining it (`components/InfoHint.tsx`).

**If we want a defensible power signal, the fix is better input, not better math**:
record results per game (or at least per match), so a card's winrate is about the
games it was actually in. That would make shrinkage and Wilson meaningful again.

Still open, and unaffected by the above:

1. **Time-series**: because runs are retained, we can chart meta evolution
   (color share, card inclusion) across runs.
2. **Archetype clustering**: vectorize decks by card membership and cluster
   (Jaccard/k-means) to auto-discover archetypes beyond the fixed tag.

**Head-to-head is intentionally out of scope** given the aggregate-record model;
if per-game matchups are wanted later, add a `matches` table and a matchup facet.

---

## 5. External integrations

### Scryfall (card data + images)
- Resolve the cube's card names in **batches via `POST /cards/collection`**
  (≤75 identifiers per request). Set a descriptive `User-Agent` + `Accept:
  application/json`, sleep ~75–100 ms between requests, exponential backoff on 429.
- Names resolve to an **exact printing**, with a search fallback for flavor names
  and double-faced cards.
- Cache full payloads in `cards.raw` (jsonb) plus extracted columns. Images used:
  `art_crop` (overlaid card fan) and `normal` (detail).
- **Images are self-hosted**, not hotlinked: `internal/images` downloads them to an
  on-disk cache (`IMAGE_CACHE_DIR`; a Docker volume at `/data/images` in prod) and
  the backend serves them from `GET /api/cards/{id}/image`. A cube sync prefetches
  the whole pool.

### Cube lists (pasted, not fetched)

The pool used to be fetched from Moxfield's unofficial deck API. **Moxfield now
blocks us**, so the adapter idea paid off: the source of truth is a **raw pasted
decklist** in `cubes.card_list`, entered by an admin at `/admin/cubes`.
`internal/moxfield` survives only to parse a `publicId` out of a URL, kept as
display metadata.

`internal/ingest.SyncCube` parses `card_list` into pool entries and fingerprints
them into `cubes.content_hash`, so an unchanged list skips re-resolution entirely.
On a change it resolves via Scryfall and diffs against `cube_cards`: new names →
insert; missing names → set `removed_at`, `is_active=false` (soft, to preserve
historical decklist references). Names that do not resolve are **dropped from the
pool but recorded** in `cube_sync_progress.unresolved`, so a typo surfaces on the
admin page instead of silently shrinking the cube.

---

## 6. Jobs & rendering pipeline

```
write (deck create/update, record update, cube sync)
      │ enqueue job (coalesced by type+cube)
      ▼
jobs worker ──▶ analytics engine recompute ──▶ new run (is_current)
      │
      └──▶ POST Next.js /api/revalidate {secret, paths[]}
                 └──▶ revalidatePath for affected decklist/user/analytics pages
```
- `jobs(id, type, payload jsonb, status, scheduled_at, attempts, last_error, dedup_key)`;
  a worker goroutine polls and coalesces on `dedup_key`, so a burst of edits = one
  recompute.
- Two job types are registered (in `cmd/server/main.go`): **`sync_cube`** and
  **`recompute_analytics`**.
- `internal/jobs/scheduler.go` re-enqueues `sync_cube` for every cube that has a
  `card_list`, every `SYNC_INTERVAL_MINUTES` (default 360), starting 30s after
  boot. Because the syncer content-hashes the list, a periodic sync on an unchanged
  cube is nearly free — its real job is to self-heal cubes whose last resolve failed.
- The image-download phase runs detached from the job and reports into
  `cube_sync_progress`, so that row — not the job's status — is what "finished"
  means to the admin UI.

---

## 7. API surface

`backend/internal/httpapi/server.go` defines every route in one `Router()` — it is
the source of truth; this list mirrors it.

Public (accepts an anonymous Caller):

```
GET  /api/health
GET  /api/users                     GET /api/users/{username}
GET  /api/cubes                     GET /api/cubes/{id}      GET /api/cubes/{id}/cards
GET  /api/cards/{slug}              GET /api/cards/{id}/image   (self-hosted cache)
GET  /api/decklists                 GET /api/decklists/{id}
GET  /api/analytics/overview|colors|cards|pairs
```

Auth — **login only; there is no register route and no public signup** (§ README):

```
POST /api/auth/login    POST /api/auth/logout    GET /api/auth/me
```

Authenticated (+ an ownership/role check inside the handler):

```
PATCH  /api/users/{id}              POST /api/users/{id}/password
POST   /api/decklists               PATCH /api/decklists/{id}
PATCH  /api/decklists/{id}/record   DELETE /api/decklists/{id}
POST   /api/decklists/infer-colors
```

Admin only:

```
DELETE /api/users/{id}              POST /api/admin/users
POST   /api/admin/cubes             PATCH /api/admin/cubes/{id}   DELETE /api/admin/cubes/{id}
POST   /api/admin/cubes/{id}/sync   GET  /api/admin/cubes/{id}/sync-status
POST   /api/admin/analytics/recompute
```

Google OAuth is **not implemented**: the `GOOGLE_*` config and the `oauth_accounts`
table exist, but no route does.

---

## 8. Deck color inference

`POST /api/decklists/infer-colors` (and on save): parse the raw list → resolve
names to `cards` → `domain.InferDeckColors` over the main board. The `decks/new`
page calls this live as the user pastes.

A deck's colors come from what it **casts**, not from what it can tap for. Only
nonland cards count, and they count by the colors of their casting cost (Scryfall
`colors`), never their `color_identity`. Color identity includes mana a card
*produces*, which is how ORing it over every card — the original rule — made a
Selesnya deck running a Mox Sapphire or a Hallowed Fountain come out blue.

Scryfall drops top-level `colors` on a multi-faced card and reports them per face,
so cast colors are the union of the faces that **have a mana cost** — both halves
of a split card, an adventure, or a modal DFC, but not the back of a transform
card, which is turned up rather than cast. Unioning every face is what made a UW
deck running Tamiyo, Inquisitive Student (whose back, Tamiyo, Seasoned Scholar, is
GU) come out with a green splash. `scryfall.castColors` does this at ingest and
`store.castColorCol` mirrors it in SQL for the recompute path; they must agree.

A color on fewer than 10% of the deck's nonland cards (`domain.SplashThreshold`,
counting copies) is a **splash**: stored apart in `decklists.splash_colors`, kept
out of `color_identity`, and excluded from every color analytic except its own
`splash_color` facet — so a GW deck with two red cards stays a two-color deck. A
deck that plays colored cards is never colorless, so when nothing clears the
threshold the best-represented color is promoted rather than splashed away.

Colors are inferred at save time, so the rule that inferred them can drift from the
current one. `store.RecomputeDeckColors` re-derives every deck in a cube from the
cached `cards` rows, and the analytics job runs it before aggregating; a deck saved
under an older rule converges on the next run without a data migration.

---

## 9. Key frontend pages

Decks live under `/decks`; the old `/decklists` paths permanently redirect. The
rendering mode is declared per page (see §1).

- `/` — redirects to the first cube's analytics. *(dynamic)*
- `/analytics` + `/analytics/[cube]` — the dense view: color winrate charts, a card
  table ranked by popularity, meta headline numbers from `meta_snapshot`. Stats are
  **scoped to a cube**. *(index dynamic; `[cube]` ISR 3600)*
- `/decks` + `/decks/[id]` + `/decks/[id]/edit` — detail page is the compact
  **overlaid card fan**: card images stacked with ~90% overlap (only the top ~10%
  name line peeks) via CSS negative margins, each linking to its Scryfall printing,
  plus record + card stats. *(index dynamic; detail ISR 3600)*
- `/decks/new` — paste list, live color inference, record entry.
- `/cubes` + `/cubes/[id]` — the pool, rendered with the same card-fan engine.
  *(index dynamic; detail ISR 300)*
- `/cards/[slug]` — card detail: printings, inclusion rate, most-played-with.
  *(ISR 300)*
- `/users/[username]` — bio + dense deck list with per-deck stats. *(ISR 3600)*
- `/login`, `/settings` (change password), `/admin/cubes` (paste + sync a cube,
  with live progress and unresolved names), `/admin/users` (create users, set
  passwords, assign deck ownership).

Charts (`ColorWinrateChart`, `RadarChart`) are **hand-rolled SVG** — there is no
chart library. Styling is one global stylesheet (`app/globals.css`, CSS custom
properties + a dark-mode block) plus inline styles; there is no Tailwind or
CSS-in-JS. Each stat carries a hover (i) explaining it (`components/InfoHint.tsx`).
