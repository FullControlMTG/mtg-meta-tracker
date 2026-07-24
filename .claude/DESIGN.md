# Design and architecture

This file absorbed the former `docs/DESIGN.md`, which has been removed.

## System shape

```
                    ┌───────────────────────────────────────────┐
                    │            Next.js (App Router)            │
  browser  ◀──────▶ │  /decks, /cubes, /analytics      dynamic   │
                    │  /decks/[id], /users/[name]      ISR       │
                    │  /analytics/[cube]               ISR       │
                    │  /decks/new         live color inference   │
                    └──────────────┬──────────────┬─────────────┘
                     rewrites /api │              │ POST /api/revalidate
                                   ▼              ▲ (on-demand render)
                    ┌──────────────────────────────┴────────────┐
                    │             Go backend (chi)               │
                    │  httpapi ─ appctx (Caller) ─ auth          │
                    │  store (pgx) ── PostgreSQL                 │
                    │  scryfall client   ingest (pasted lists)   │
                    │  jobs worker ── analytics engine           │
                    └──────────────┬────────────────────────────┘
                                   ▼
                             Scryfall API
```

Next.js owns rendering and UX; Go owns data, auth, external integrations, and
analytics. They speak JSON over `/api`, proxied by a Next.js rewrite so the
session cookie stays same-site and there is no CORS layer.

## Components

| Component | Responsibility |
|---|---|
| Next.js frontend | All rendering and UX. Proxies `/api/*` to the backend via a rewrite. |
| Go API (`httpapi`) | Routes, request auth, JSON. `server.go` is the route source of truth. |
| `store` | All SQL, hand-written, over pgx. Owns `schema.sql`. |
| `ingest` | Cube pool sync: parse pasted list, resolve, diff `cube_cards`. |
| `decklist` | Decklist parsing and per-card resolution against the pool. |
| `scryfall` | Batched card client (75 identifiers per request), rate-limited. |
| `images` | Downloads and serves card images from a local cache. |
| `analytics` | Pure `aggregate()` over deck rows plus a persistence/promotion engine. |
| `jobs` | Postgres-backed queue, a polling worker, and a periodic scheduler. |
| `revalidate` | Calls the frontend's revalidation webhook after a recompute. |

## Data flow

```
admin pastes cube list
        │
        ▼
  cubes.card_list ──hash──▶ unchanged? stop
        │ changed
        ▼
  scryfall.Resolve (batched)  ──unresolved──▶ cube_sync_progress.unresolved
        │
        ▼
  cube_cards (soft-delete absent rows)  ──▶ enqueue recompute_analytics
                                                    │
player uploads deck                                 │
        │                                           │
        ▼                                           │
  decklist.ParseList → resolve vs pool → infer colors│
        │                                           │
        ▼                                           ▼
  decklists + decklist_cards ──enqueue──▶ jobs worker (2s poll)
                                                    │
                                                    ▼
                                    store.RecomputeDeckColors (whole cube)
                                                    │
                                                    ▼
                                    analytics.aggregate (pure)
                                                    │
                                                    ▼
                              analytics_runs + color_stats, color_trend_stats,
                              card_stats, card_pair_stats, meta_snapshot,
                              deck_metric_stats  → promote to is_current
                                                    │
                                                    ▼
                              POST /api/revalidate (shared secret) → revalidatePath
```

## The request Caller

Every backend operation receives a `context.Context` carrying a `Caller`, built by
auth middleware from the session cookie, or absent for public calls.

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

Authorization lives in small predicates the handlers consult, e.g.
`caller.CanMutateOwned(deck.UserID)` = `Role == Admin || deck.UserID == caller.UserID`.
Read endpoints accept `Public`; create/update/delete require `Authenticated` plus
an ownership or role check. One auditable place per rule.

## Data model

Color identity is a 5-bit bitset (`SMALLINT`, 0–31): `W=1, U=2, B=4, R=8, G=16`;
colorless = 0.

| Table | Purpose | Key columns |
|---|---|---|
| `users` | accounts | `id, username, email, display_name, bio, avatar_url, role, password_hash (nullable)` |
| `oauth_accounts` | Google links (table only — OAuth is unimplemented) | `user_id, provider, provider_account_id` |
| `sessions` | server-side sessions, revocable | `id (token), user_id, expires_at` |
| `cubes` | a card pool = one pasted list | `id, name, card_list (raw paste, source of truth), content_hash, moxfield_public_id (display only), last_synced_at` |
| `cards` | Scryfall cache | `scryfall_id (pk), oracle_id, name, slug (generated), cmc, type_line, colors, color_identity, rarity, image_*, raw jsonb` |
| `cube_cards` | pool membership + history | `cube_id, card_id, quantity, added_at, removed_at (nullable), is_active` |
| `cube_sync_progress` | live progress for the admin sync UI | `cube_id (pk), status, cards_total, images_total/done/failed, unresolved text[]` |
| `decklists` | deck + metadata + record | see below |
| `decklist_cards` | normalized deck contents | `decklist_id, card_id, card_name, quantity, is_resolved, board` |

`cards.slug` is a **generated** `STORED` column powering `/cards/<slug>`, so it
cannot drift from the name. It is not unique — two printings of a name are two
rows — so slug lookups tie-break rather than assume one hit.

`cube_sync_progress.unresolved` holds names Scryfall could not resolve on the last
sync. They are dropped from the pool, so surfacing them is what keeps a typo from
silently shrinking the cube.

### `decklists`

Metadata, the list, and the record all live in one table; the record is nullable
and added after the fact.

```
id, cube_id, user_id            -- uploader; owner + admins get update/delete
name, description
color_identity    smallint      -- inferred bitset (splashes excluded)
splash_colors     smallint      -- inferred bitset of sub-threshold colors
archetype         text null     -- CHECK enum: aggro|control|midrange|tempo|combo
source_url        text null
decklist_raw      text          -- the raw pasted list
card_count        int
status            text          -- draft | active | archived
played_at         date          -- the day it was played; defaults to today
-- record (nullable, added later; updating these triggers a recompute)
games_played, wins, losses      -- int, default 0
event_name        text null
record_updated_at timestamptz null
winrate           numeric GENERATED ALWAYS AS
                    (CASE WHEN games_played>0 THEN wins::numeric/games_played END) STORED
created_at, updated_at
```

`decklist_cards` is the backbone of card-level analytics — one row per card, with
`card_id` resolved against the cube pool (`is_resolved = false` when a name cannot
be matched, so import problems are visible without losing data).

## Analytics schema

Every stat table is keyed by `run_id`. `analytics_runs(id, cube_id, trigger,
status, decks_included, games_included, is_current, started_at, finished_at)`
tracks executions; `is_current` marks the latest good run per cube, and pages read
`WHERE is_current`. A unique partial index enforces at most one current run per
cube.

### `color_stats` — four facets in one table

```
color_stats(run_id, facet, facet_key, deck_count, games, wins, losses, winrate)
```

- `exact_identity` → `facet_key` = the 0–31 bitset (WUBRG combos).
- `single_color` → `facet_key` = one color bit (decks *containing* W…).
- `color_count` → `facet_key` = 0–5 (mono / two-color / …).
- `splash_color` → `facet_key` = one color bit (decks *splashing* W…).

Answers "do blue decks win more?", "is two-color better than five-color?", and
per-combo winrates. `splash_color` is the **only** place splashes are counted;
they are excluded from the other three, so a GW deck with two red cards does not
read as a three-color deck.

### `color_trend_stats` — the color pie over time

```
color_trend_stats(run_id, as_of, color, deck_count, total_decks, share)
```

The one time series, and the only consumer of `decklists.played_at`. One row per
(day a deck was played, color), with all five colors present on every day —
including those at zero, so an area band has a point at every x.

Cumulative: a row counts every deck dated on or before `as_of`, making the series
a meta trend rather than a per-night sample of whoever happened to build that
evening. `share` is normalized across the five colors, **not** against
`total_decks` — a two-color deck plays two colors, so the counts sum past the deck
total and would never stack to 100%. `total_decks` rides along so a reader can be
told "6 of 11 decks" rather than only a percentage. Splashes are excluded.

Days nobody played are absent rather than repeated: nothing changed, and the
straight line the chart draws between two points says that better than a row per
empty day would.

### `card_stats`

```
card_stats(run_id, card_id, deck_count, inclusion_rate, games, wins, losses, winrate)
```

Popularity is `inclusion_rate` (`deck_count / total_decks`), which is the table's
sort order. Performance is `winrate`, read alongside `deck_count` and `games`.

Note what `winrate` is *not*: a card's games are its **deck's** games, attributed
wholesale — every card in a deck inherits that deck's full record. This is the
attribution behind the removed statistics.

### `card_pair_stats`

```
card_pair_stats(run_id, card_a_id, card_b_id, co_count, pair_winrate)
```

Powers "most often played with" on a card page. Stored only for pairs with
`co_count >= 2` to bound the n² blow-up, both directions per pair, ranked by
`co_count` descending. Because the ranking is raw co-occurrence, the list skews
toward cube staples — it answers "what usually shares a deck with this card", not
"what is *specifically* associated with it".

### `meta_snapshot` and `deck_metric_stats`

```
meta_snapshot(run_id, total_decks, total_games, overall_winrate,
              avg_cmc, avg_color_count, mono_share, multi_share,
              power9_share, undefeated_decks, …)

deck_metric_stats(run_id, metric, bucket, deck_count, winrate)
```

Headline meta numbers, plus winrate by bucket (e.g. `metric='avg_cmc'`) so the
dashboard can chart "does a lower curve win?".

`power9_share` is the fraction of decks running at least one of the Power Nine,
matched by card name against a hard-coded list in `analytics.powerNine` — the
nine cards are the definition, so there is nothing on a card to derive it from.
`undefeated_decks` counts decks with at least one game played and no losses; a
deck with no record has not gone undefeated, it has not played.

Basic lands are excluded from `card_stats` and `card_pair_stats` — every deck
plays them, so they would top inclusion and co-occur with everything — and all
lands are excluded from mana-value averages.

## Jobs pipeline

```
write (deck create/update, record update, cube sync)
      │ enqueue job (coalesced by dedup_key)
      ▼
jobs worker ──▶ analytics recompute ──▶ new run promoted to is_current
      │
      └──▶ POST /api/revalidate {secret, paths[]} ──▶ revalidatePath
```

- `jobs(id, type, payload jsonb, status, scheduled_at, attempts, last_error,
  dedup_key)`. A worker goroutine polls every 2s and coalesces on `dedup_key`, so
  a burst of edits produces one recompute.
- Two job types are registered in `cmd/server/main.go`: `sync_cube` and
  `recompute_analytics`.
- `internal/jobs/scheduler.go` re-enqueues `sync_cube` for every cube with a
  `card_list`, every `SYNC_INTERVAL_MINUTES` (default 360), starting 30s after
  boot. Because the syncer content-hashes the list, a periodic sync on an
  unchanged cube is nearly free — its real job is to self-heal cubes whose last
  resolve failed.
- The image-download phase runs detached from the job and reports into
  `cube_sync_progress`, so that row — not the job's status — is what "finished"
  means to the admin UI.

## API surface

`backend/internal/httpapi/server.go` is the source of truth. Public:

```
GET  /api/health
GET  /api/users                     GET /api/users/{username}
GET  /api/cubes                     GET /api/cubes/{id}   GET /api/cubes/{id}/cards
GET  /api/cards/{slug}              GET /api/cards/{id}/image   (self-hosted cache)
GET  /api/decklists                 GET /api/decklists/{id}
     (?cube= ?user= ; each item carries its owner and cube name for the
      deck table's `user:` / `cube:` filters)
GET  /api/analytics/overview|colors|color-trend|cards|pairs
GET  /api/today                     (the server's date, in APP_TIMEZONE)
```

Auth — login only; there is no register route and no public signup:

```
POST /api/auth/login    POST /api/auth/logout    GET /api/auth/me
```

Authenticated, with an ownership/role check inside the handler:

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

Google OAuth is **not implemented**: the `GOOGLE_*` config and the
`oauth_accounts` table exist, but no route does.

## Frontend pages

Decks live under `/decks`; the old `/decklists` paths permanently redirect.

- `/` — redirects to the first cube's analytics. *(dynamic)*
- `/analytics` + `/analytics/[cube]` — the dense view: color charts, the color
  trend, a card table ranked by popularity, headline numbers from
  `meta_snapshot`. Scoped to a cube. *(index dynamic; `[cube]` ISR 3600)*
- `/decks` + `/decks/[id]` + `/decks/[id]/edit` — the index is the filterable deck
  table, and takes a query in the URL (`?q=`, `?sort=`, `?dir=`); the detail page
  renders the overlaid card fan, each card linking to its Scryfall printing, plus
  record and card stats. *(index dynamic; detail ISR 3600)*
- `/decks/new` — paste a list, live color inference, record entry.
- `/cubes` + `/cubes/[id]` — the pool, same card-fan engine. *(index dynamic;
  detail ISR 300)*
- `/cards/[slug]` — printings, inclusion rate, most-played-with. *(ISR 300)*
- `/users/[username]` — bio, that player's own stats (headline tiles, colors
  played and splashed as radars, a color-pairing heatmap, their combinations
  ranked), then a dense deck list. The stats are computed in the page from the
  player's decklists, not from `meta_snapshot`: those are per-cube aggregates
  over everybody, and a player plays across cubes. *(ISR 3600)*
- `/login`, `/settings` (change password), `/admin/cubes` (paste and sync a cube,
  with live progress and unresolved names), `/admin/users` (create users, reset
  passwords).

## Key decisions

### Analytics are precomputed, not queried live

Any write that affects aggregates enqueues a `recompute_analytics` job keyed
`recompute:<cubeID>`, which dedupes concurrent triggers. The worker recomputes
every snapshot for the cube, writes a new `analytics_runs` row, and promotes it to
`is_current`; pages read the current run.

Trade-off: the whole cube is recomputed for a single deck edit. At this scale that
is cheap, and it makes every snapshot mutually consistent — no partially-updated
run is ever visible.

### Rendering is chosen per page

Index pages (`/`, `/decks`, `/cubes`, `/analytics`) are `force-dynamic`. Detail
pages are ISR: `revalidate = 3600` for decks, users, and cube analytics; `300` for
cards and cube pools. After a recompute the backend calls the frontend's
`/api/revalidate` with a shared secret.

Rejected: making index pages static. They were prerendered during `next build`,
where the backend does not exist, so they shipped empty and served that for a full
revalidate window (fixed in `f9361fe`).

Consequence: with `REVALIDATE_URL` unset — the default in dev — the webhook is a
silent no-op and pages fall back to their ISR window. A page that looks stale in
dev is usually this, not a slow job.

### Schema as boot-time idempotent SQL

`schema.sql` is embedded in the binary and re-applied by `store.EnsureSchema` on
every startup. There is no migration tool, directory, or version table.

Rejected alternatives, both actually tried: a separate migration container
(`efcd26a` replaced it with a committed dump) and a `db/schema.sql` applied by
Postgres on first init (`dcdda9a` moved to the current model). The current model
means a fresh database and an existing one converge without an ordering problem.

The cost is that every statement must be idempotent, and a failing one stops the
server from booting. Rules:

- New tables/indexes: `CREATE TABLE/INDEX IF NOT EXISTS`.
- New columns: `ALTER TABLE … ADD COLUMN IF NOT EXISTS`, in the *Idempotent
  migrations for existing databases* section at the bottom — and also in the
  `CREATE TABLE` block, which covers fresh databases only.
- New CHECK constraints: normalize existing rows, then `DROP CONSTRAINT IF EXISTS`
  before `ADD CONSTRAINT` (Postgres has no `ADD CONSTRAINT IF NOT EXISTS`). The
  archetype enum is the worked example.
- Tightening to `NOT NULL`: backfill first, then `ALTER COLUMN … SET NOT NULL`.
  `played_at` is the worked example.
- A backfill re-runs forever. It is safe only when a later statement makes it
  unmatchable. Prefer a rule that re-derives itself — see below.

### Recompute rather than backfill

Deck colors are inferred at save time, so stored values can drift from the current
rule. `store.RecomputeDeckColors` re-derives every deck in a cube from the cached
`cards` rows (no Scryfall calls), and the analytics job runs it before
aggregating. A rule change therefore converges on the next run.

Consequence: changing a color rule needs no data migration, but it does need both
copies of the rule changed — the Go one at ingest and the SQL one for recompute.

### Colors come from casting cost, not color identity

A deck's colors are the colors in the casting costs of its nonland cards (Scryfall
`colors`), never `color_identity`. Color identity counts mana a card *produces*,
which made a Selesnya deck running a Mox Sapphire come out blue (`3fe29c9`).

Scryfall omits top-level `colors` on multi-faced cards and reports them per face,
so cast colors union only faces that **have a mana cost**: both halves of a split
card, an adventure, and a modal DFC count; a transform card's back does not,
because it is turned up rather than cast.

A color on under 10% of a deck's nonlands (`domain.SplashThreshold`) is a splash:
stored in `decklists.splash_colors`, excluded from `color_identity` and from every
color analytic except the `splash_color` facet.

### Cube pools are pasted lists, fingerprinted

Moxfield's API blocks the app (`c229182`), so `cubes.card_list` is the source of
truth. `content_hash` fingerprints the entry set — name, printing, and quantity —
so an unchanged list skips resolution entirely. `resolverVersion` is folded into
the hash and bumped whenever resolution changes what a given list produces,
invalidating every stored fingerprint on deploy.

Unresolvable names are recorded in `cube_sync_progress.unresolved` and shown on
the admin page rather than silently shrinking the pool (`7a21be0`, `10a2f83`).

### Self-hosted card images

Images are downloaded on miss and served from `/api/cards/{id}/image`
(`894883d`). Production uses a named Docker volume rather than a bind mount so
Docker seeds it from the image with the backend's UID already set — a bind mount
lands root-owned and unwritable (`226efd3`, and the earlier fixes `2f1770f`,
`be44537`).

### Dates are the server's, in the playgroup's timezone

`decklists.played_at` is a deck field, not part of the win/loss record; the record
PATCH deliberately does not touch it, so entering a record later cannot re-date a
deck. "Today" means today in `config.Timezone` (`APP_TIMEZONE`, default
`America/Los_Angeles`), because the container runs in UTC where the day rolls over
mid-afternoon locally. `GET /api/today` exposes it so a date picker opens on the
same day the backend would choose. `time/tzdata` is compiled into the binary
because the deploy image has no `/usr/share/zoneinfo`.

Dates cross the wire as `"2006-01-02"` inbound and midnight-UTC RFC3339 outbound.
The frontend reads the calendar day off the string (`isoDay`/`fmtDate`) because
`new Date("…T00:00:00Z")` renders the previous day west of Greenwich.

### The deck list filters with a query language, client-side

`DeckTable` is the one deck-list element, and every page that shows decks uses it.
Its filter is a line of text in the `field:value` language of `lib/deckQuery.ts` —
`losses:0 games>0 c:ur` — parsed to a predicate over the list the page already
fetched. Terms are ANDed, `-` negates one, quotes group a value, and a bare word
searches the deck name. The grammar is deliberately GitHub's and Scryfall's rather
than one invented here, since the playgroup types Scryfall queries anyway.

The extensibility is the `FIELDS` table, not boolean algebra: a filterable field is
one entry, and it is then parseable, documentable (the panel's reference is
generated from it) and linkable for free. There is no OR and no parentheses.

Two consequences worth knowing:

- The filter runs in the browser over the whole list, so the backend keeps serving
  one cacheable payload per page and the filter is instant. This holds because a
  playgroup's deck count is in the hundreds. A cube with tens of thousands of decks
  would want this pushed into `store.DecklistFilter` instead.
- `GET /api/decklists` therefore denormalizes the owner (`user`) and the cube
  (`cube_name`) onto each item — `user:` and `cube:` have nothing to match against a
  bare uuid. Both are read whole and mapped in the handler rather than joined per
  deck.

Filtering is opened from a button in the table's toolbar, which hosts the page's own
heading and buttons so the control costs no vertical space. A query in the URL
(`?q=`, plus `?sort=`/`?dir=`) is applied on the server for first paint and opens the
panel, so a list that arrives filtered shows what filtered it; the live query is
mirrored back with `history.replaceState` rather than a router push, since re-running
the server component would refetch the same payload to render the same rows. This is
how a stat links to the rows behind it — the "Undefeated decks" tiles on the cube and
player pages link to the same decks they counted, best record first, from the shared
`UNDEFEATED_TERMS`.

### No chart library

Charts are hand-rolled SVG (`ColorWinrateChart`, `RadarChart`, `ColorTrendChart`)
or CSS grid (`ColorPairHeatmap`). The MTG palette is semantic and cannot be
re-picked for contrast — white is a near-white and black a near-black — so every
fill carries a `--pip-ring` outline, and charts add a legend, direct labels, and a
tooltip naming every series so identity never rests on hue alone.

The one place the mana colors are *not* used is `ColorPairHeatmap`: its cells
encode a count, so they take a single-hue `--accent` ramp and let the axes carry
the colors being crossed. The ramp stops well short of solid so the number printed
in each cell keeps its contrast on both surfaces.

`ColorTrendChart` stacks its bands by where each color stands on the most recent
point, largest on top, rather than in fixed WUBRG order — the last point is the
one a reader came for. Legend and tooltip follow the same order.

## Dependencies

Backend: `go-chi/chi/v5`, `google/uuid`, `jackc/pgx/v5`, `golang.org/x/crypto`,
`golang.org/x/sync`. Nothing else direct.

Frontend: `next`, `react`, `react-dom` at runtime; `typescript` and `@types/*` in
dev. No UI, styling, charting, or data-fetching library.

External: Scryfall only.
