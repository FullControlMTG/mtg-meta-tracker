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
| `store` | All SQL, hand-written, over pgx. Owns `migrations/`. |
| `ingest` | Cube pool sync: parse pasted list, resolve, diff `cube_cards`. |
| `decklist` | Decklist parsing and per-card resolution against the pool. |
| `scryfall` | Batched card client (75 identifiers per request), rate-limited. |
| `images` | Downloads and serves card images from a local cache. |
| `analytics` | Pure `aggregate()` over deck rows plus a persistence/promotion engine. |
| `jobs` | Postgres-backed queue, a polling worker, and a periodic scheduler. |
| `dbrestore` | Admin restore API's runner: lists dumps, runs one `store.RestoreDump` in the background with the worker paused. |
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
| `cubes` | a card pool = one pasted list; selects its decks by tag | `id, name, owner_id (fk users, SET NULL), card_list (raw paste, source of truth), deck_tag (generated `cube:<id>`, unique), content_hash, moxfield_public_id (display only), last_synced_at` |
| `cube_members` | who may see a cube (the read boundary) | `cube_id, user_id, joined_at`, PK `(cube_id, user_id)`; owner is always a member |
| `cube_invites` | pending/answered cube-join invites | `id, cube_id, invitee_id, invited_by, status (pending\|accepted\|declined), UNIQUE (cube_id, invitee_id)` |
| `cards` | Scryfall cache | `scryfall_id (pk), oracle_id, name, slug (generated), cmc, type_line, colors, color_identity, rarity, image_*, raw jsonb` |
| `cube_cards` | pool membership + history | `cube_id, card_id, quantity, added_at, removed_at (nullable), is_active` |
| `cube_sync_progress` | live progress for the admin sync UI | `cube_id (pk), status, cards_total, images_total/done/failed, unresolved text[]` |
| `decklists` | deck + metadata + record; the base unit, owned by no cube | see below |
| `decklist_cards` | normalized deck contents | `decklist_id, card_id, card_name, quantity, is_resolved, board` |
| `combos` | an admin-named set of cards, per cube | `id, cube_id, name, description`, `UNIQUE (cube_id, name)` |
| `combo_cards` | the pieces of one combo | `combo_id, card_id, position` |

`cards.slug` is a **generated** `STORED` column powering `/cards/<slug>`, so it
cannot drift from the name. It is not unique — two printings of a name are two
rows — so slug lookups tie-break rather than assume one hit.

`cube_sync_progress.unresolved` holds names Scryfall could not resolve on the last
sync. They are dropped from the pool, so surfacing them is what keeps a typo from
silently shrinking the cube.

### `decklists`

Metadata, the list, and the record all live in one table; the record is nullable
and added after the fact. A deck stands alone: it is filed under cubes (and, later,
other collections) by its tags, not by a foreign key — see
[Decks are the unit; cubes select them by tag](#decks-are-the-unit-cubes-select-them-by-tag).

```
id, user_id                     -- uploader; owner + admins get update/delete
format            text null     -- game format, lower-case; 'cube' for every deck today
tags              text[]        -- collections it is filed under, e.g. cubes.deck_tag; GIN-indexed
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
- Three job types are registered in `cmd/server/main.go`: `sync_cube`,
  `recompute_analytics` and `prune` (retention, below).
- **Retries.** A handler error re-queues the job (`pending`, `scheduled_at`
  pushed back 1m, then 2m) until it has run `jobs.MaxAttempts` (3) times; then it
  stays `failed` with `last_error`. A job whose type has no handler fails at once.
  If a newer pending job holds the same `dedup_key`, the failed one is not
  re-queued — the newer one covers it, and the unique pending index forbids two.
- **Recovery.** At boot, before the worker starts, `store.RecoverJobs` returns
  rows a restart left `running` to `pending`. One already at the attempt cap, or
  superseded by a pending job with its key, is marked `failed` instead, so a job
  that kills the process cannot crash-loop it.
- `internal/jobs/scheduler.go` re-enqueues `sync_cube` for every cube with a
  `card_list`, plus one `prune` job, every `SYNC_INTERVAL_MINUTES` (default 360),
  starting 30s after boot. Because the syncer content-hashes the list, a periodic sync on an
  unchanged cube is nearly free — its real job is to self-heal cubes whose last
  resolve failed.
- The image-download phase runs detached from the job and reports into
  `cube_sync_progress`, so that row — not the job's status — is what "finished"
  means to the admin UI.

## API surface

`backend/internal/httpapi/server.go` is the source of truth. Public (no session):

```
GET  /api/health          GET /api/cards/{id}/image   (self-hosted cache)
GET  /api/cards/sample    GET /api/today              (server date, in APP_TIMEZONE)
POST /api/auth/login      POST /api/auth/logout        GET /api/auth/me
```

Authenticated — cube reads are membership-gated, writes owner/role-gated, inside the
handlers (see `httpapi/access.go`: `requireCubeAccess` / `requireCubeOwner`):

```
GET    /api/users                  GET /api/users/{username}
PATCH  /api/users/{id}             POST /api/users/{id}/password
GET    /api/cubes                  (only the caller's cubes; admin sees all)
POST   /api/cubes                  (any user; creator becomes owner + member)
GET    /api/cubes/{id}             PATCH /api/cubes/{id}   DELETE /api/cubes/{id}   (owner/admin)
GET    /api/cubes/{id}/cards       GET /api/cubes/{id}/combos
POST   /api/cubes/{id}/sync        GET /api/cubes/{id}/sync-status                  (owner/admin)
POST   /api/cubes/{id}/combos      PATCH /api/combos/{id}   DELETE /api/combos/{id} (owner/admin)
GET    /api/cubes/{id}/members     DELETE /api/cubes/{id}/members/{userId}          (owner/admin)
GET    /api/cubes/{id}/invites     POST /api/cubes/{id}/invites                     (owner/admin)
GET    /api/me/invites             POST /api/invites/{id}/accept|decline           (invitee)
GET    /api/cards/{slug}?cube=     (membership-gated)
GET    /api/decklists?cube=[&user=]   GET /api/decklists/{id}   (cube required + gated)
POST   /api/decklists              PATCH /api/decklists/{id}
PATCH  /api/decklists/{id}/record  DELETE /api/decklists/{id}   POST /api/decklists/infer-colors
GET    /api/analytics/overview|colors|color-trend|cards|pairs   (?cube= required + gated)
```

Admin only (site administration): `DELETE /api/users/{id}`, `POST /api/admin/users`,
`POST /api/admin/analytics/recompute`.

Google OAuth is **not implemented**: only the `oauth_accounts` table exists. The
`GOOGLE_*` config and Jenkins credentials were removed until a route needs them.

## Frontend pages

The app is scoped to a cube: everything cube-specific lives under `/cube/[id]/…`, and
the cube's tab bar (Overview / Cards / Decks / Manage) is on the cube *layout*
(`app/cube/[id]/layout.tsx`), not the global nav. The layout fetches the cube, which
404s a non-member — that fetch is the access gate.

- `/` — logged-in **dashboard**: the caller's cubes, pending invites (accept/decline),
  and "new cube". Anonymous: the marketing landing. *(client)*
- `/cube/[id]` — the cube **overview** (color charts, the color trend, card table,
  headline `meta_snapshot` numbers). *(ISR 3600)*
- `/cube/[id]/cards` + `/cube/[id]/cards/[slug]` — the pool (card-fan engine), and per-
  card detail (inclusion, most-played-with, decks playing it). *(ISR 300)* The pool and
  a deck's cards share `CardBrowser`, which groups (color / mana value / type / board /
  none; `groupCardsBy`) and then sorts within each group (color / mana value / name /
  type, either direction; `sortCardsBy`), both in `lib/colors.ts`, both client-side and
  neither in the URL. The pages pass the cards ungrouped, so the API response is the same
  whatever the reader picks — the seam a later card-level cache sits behind. The pool
  opens grouped by color, a deck by board; only a deck offers the board grouping.
- `/cube/[id]/decks` + `/decks/new` + `/decks/[deckId]` + `/decks/[deckId]/edit` — the
  filterable deck table (`?q=/?sort=/?dir=`), create, detail (card fan + combos), edit.
  The owner picker lists the cube's **members**. *(list dynamic; detail ISR 3600)*
- `/cube/[id]/manage` — owner/admin only: cube settings + pool sync, members + invites,
  combos. *(client)*
- `/users/[username]?cube=` — the player's stats and decks **within one cube** (radars,
  the 31-combination wheel, a deck list). Without `?cube=` it is just the profile.
- `/login`, `/settings` (change password), `/admin/users` (site admin: create users,
  reset passwords).

## Key decisions

### The cube is the data boundary

Every user is logged in (the app has no public read), but what they *see* is scoped
to cube membership. `cubes.owner_id` is who administers a cube; `cube_members` is who
may read it (owner included); site admins bypass both. A cube's decks, analytics,
cards, and combos are all membership-gated — a non-member gets 404 (existence hidden),
a member-non-owner gets 403 on owner actions. Gating is per-handler via
`requireCubeAccess` / `requireCubeOwner` (`httpapi/access.go`), not middleware, because
the cube id arrives variously in the path (`/cubes/{id}/…`) or a `?cube=` query.

Any logged-in user can create a cube (creator = owner + first member); owners manage
their own cube's list/sync/combos/members. The `admin` role is now only site
administration (users, manual recompute). Invites are in-app (there is no email and no
public signup, so the invitee already has an account): owner invites by username →
`cube_invites` pending row → the invitee accepts on their dashboard, which enrolls them.

Membership is opt-in: an invite is the only way in besides creating the cube.
No migration may enroll users. The original backfill seeded every user into any
cube with no members; because the schema then re-ran on every boot, a cube whose
sole owner was deleted was opened to all users on the next deploy, so that seed was
removed. The baseline's remaining backfill hands an orphaned cube's *ownership* to
the first admin, which grants no membership. It now runs once, so a cube orphaned
later keeps `owner_id` NULL; admins bypass the owner check anyway.

### Decks are the unit; cubes select them by tag

Decks used to belong to one cube through `decklists.cube_id` and were deleted with
it. Since migration `0002_decklist_centric` a deck is the base unit and a cube is
metadata that *selects* decks: each cube has a generated, unique
`deck_tag = 'cube:' || id`, and its decks are `d.tags @> ARRAY[c.deck_tag]`
(`store.deckInCube`, the one place that spells it). Everything else a cube has —
pool, members, invites, combos, analytics runs — is still keyed by `cube_id`.

- A deck can carry several cubes' tags and then appears in each. The API still
  creates a deck into exactly one cube (`cube_id` in the body adds that cube's
  tag), and serves `cube_id` as a derived field: the oldest cube whose tag the
  deck carries, absent if none does.
- Deleting a cube no longer deletes its decks. A deck under no cube is readable
  only by its owner and admins (`requireDeckAccess`), and its list cannot be
  re-resolved (409), since names resolve against a cube's pool.
- `deck_tag` is generated so it cannot drift and needs no backfill, on migrate or
  restore. A collection that wants a richer filter than one tag should get its own
  column when it exists, not a reinterpretation of this one.

### Analytics are precomputed, not queried live

Any write that affects aggregates enqueues a `recompute_analytics` job keyed
`recompute:<cubeID>`, which dedupes concurrent triggers. The worker recomputes
every snapshot for the cube, writes a new `analytics_runs` row, and promotes it to
`is_current`; pages read the current run.

Trade-off: the whole cube is recomputed for a single deck edit. At this scale that
is cheap, and it makes every snapshot mutually consistent — no partially-updated
run is ever visible.

### Combos are matched on read, not stored per deck

A deck's colors are inferred once and written to its row; its combos are not.
`store.MatchCombos` runs the match every time a deck is read, against the cube's
current definitions — the same call the live inference preview makes while a list
is being typed, so the form and the saved page cannot disagree.

The reason is that combos are configured after the fact. An admin adds "Thoracle"
long after the decks that play it were uploaded, and a stored answer would leave
every one of them wrong until each was saved again — the problem
`RecomputeDeckColors` exists to clean up for colors. Reading is cheap enough to
make that unnecessary: a deck holds forty cards and a cube a handful of combos.

Pieces are matched on `oracle_id`, not `scryfall_id` (`store.comboKeyExpr`): a
combo names one printing and a deck resolves its own, so matching on the printing
would miss decks that genuinely run the combo. Only the mainboard counts, as with
colors and card stats — a piece in the sideboard is not a combo the deck can
assemble. Editing a definition revalidates every deck page in that cube, which is
what keeps ISR from serving the old answer for an hour.

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

### Schema as versioned migrations

Schema changes are numbered files in `backend/internal/store/migrations/`
(`NNNN_description.sql`), embedded in the binary. At startup `store.Migrate` takes a
Postgres advisory lock, then applies each file not yet recorded in
`schema_migrations (version, name, applied_at)`, in version order, each in its own
transaction together with its `schema_migrations` row. A failing file rolls back,
nothing after it runs, and the server exits before it listens. On an empty database
`store.Migrate` builds the latest schema; it is the entry point for anything that
needs one. The restore path applies the same files inside its own transaction
([Flashing the database from a dump](#flashing-the-database-from-a-dump)).

`0001_baseline.sql` is the old `schema.sql`, frozen. Before migrations, that file
was re-applied on every boot, so every statement had to be idempotent and every
backfill re-ran forever. That is how a one-time membership seed went on to open
private cubes. Because the baseline is still idempotent, a database it built (prod
included) accepts it as `0001` with no data change.

Earlier models, both tried and dropped: a separate migration container (`efcd26a`
replaced it with a committed dump), and a `db/schema.sql` applied by Postgres on
first init (`dcdda9a` moved to re-applying `schema.sql` from the binary).

Rules:

- Never edit a file that has shipped. Change the schema in the next-numbered file.
  Version numbers must be unique; the loader rejects a misnamed or duplicate file
  rather than skipping it.
- A file runs once, so it may be non-idempotent: plain `ALTER TABLE … ADD COLUMN`,
  `DROP COLUMN`, one-shot backfills.
- It runs inside a transaction, so nothing that refuses one (`CREATE INDEX
  CONCURRENTLY`, `VACUUM`).
- It runs against prod data. Backfill before tightening to `NOT NULL` or adding a
  `CHECK`.
- Upgrading a database built by the pre-migration `schema.sql` is covered by
  `TestMigrateUpgradesPreMigrationDatabase` (its fixture is
  `store/testdata/schema_pre_migrations.sql`).

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

Charts are hand-rolled SVG (`ColorWinrateChart`, `RadarChart`, `ColorTrendChart`,
`ColorWheelGrid`) or CSS grid. The MTG palette is semantic and cannot be re-picked
for contrast — white is a near-white and black a near-black — so every fill carries
a `--pip-ring` outline, and charts add a legend, direct labels, and a tooltip
naming every series so identity never rests on hue alone.

Where a mark encodes a *count* rather than an identity it takes a single-hue
`--accent` ramp instead of the mana colors, stopping well short of solid so the
number printed over it keeps its contrast on both surfaces. `ColorWheelGrid` runs
both at once: the wheel glyph is identity (mana colors, or `--grid` for a
combination never built), and the cell behind it is magnitude (the `--accent`
ramp). The glyph carries its own `--surface` disc so it reads the same over any
tint.

### All 31 color combinations, not just the pairs

`ColorWheelGrid` replaced a 5×5 pairing heatmap, which had the wrong shape: a pair
is a Cartesian product, but a deck's colors are a *subset*, so every deck above two
colors dissolved into unrelated pair cells. The grid comes back via the pentagon —
rotating the color pie by a fifth maps each *kind* of combination onto itself, so
mono, allied pair, enemy pair, shard, wedge and four-color are each an orbit of
exactly five, plus WUBRG alone: 6 × 5 + 1 = 31, every combination once. Rows are
the anchoring color, columns are the family, and since the families run 1, 2, 2, 3,
3, 4 colors deep the column axis is also the color-count axis. A row is therefore
not "every deck containing white" — that question belongs to the Colors Played
radar. The component's header comment carries the full derivation.

`ColorTrendChart` stacks its bands by where each color stands on the most recent
point, largest on top, rather than in fixed WUBRG order — the last point is the
one a reader came for. Legend and tooltip follow the same order.

## Deployment

Jenkins deploys `main` to one host with `docker compose` (`Jenkinsfile`,
`docker-compose.yml`). Checkout, Preflight, Lint & Type-check, Integration Tests
and Prepare Environment run on every branch; everything after them is one `Deploy` stage
group behind `isMain()`. That helper accepts `BRANCH_NAME` (multibranch jobs) or
`GIT_BRANCH` (`origin/main`, plain pipeline jobs), because a bare
`when { branch 'main' }` reads only `BRANCH_NAME` and would skip every deploy in a
plain job.

Inside `Deploy`, in order:

1. **Build Images**: `docker compose build`, while the old deployment still
   serves. A broken build fails here and prod stays up.
2. **Backup Database**: a `pg_dump -Fc` of the running prod database (see below).
3. **Teardown**: `docker compose down --remove-orphans`, never `-v`.
4. **Start**: `docker compose up -d --no-build`, which uses the images from step 1.
5. **Health Check**, then **Smoke Test** (`/api/health`).

### Pre-deploy database dumps

The dump runs before Teardown because the new backend applies its schema when it
boots. That makes the dump the last copy of the data from before the change.

- **Where.** The `/pwspool/software/mtg-meta-tracker/dump/` bind mount, a sibling
  of `postgres/`. It is never inside `postgres/`, which is Postgres' own data
  directory.
- **Name.** `mtg_meta_<UTC yyyyMMdd'T'HHmmss'Z'>_<git short sha>.dump`, for
  example `mtg_meta_20261115T081737Z_f8a625a.dump`. Each name records when the
  dump was taken and which commit was about to deploy, and the names sort by time.
- **Format.** Postgres custom format (`-Fc`), so `pg_restore` reads it whatever
  version wrote it, and can restore a single table.
- **How.** `docker compose run --rm -T db-dump` starts an ephemeral
  `postgres:16-alpine` on `appnet` (the "tools" profile keeps `up` from starting
  it), so the Jenkins agent needs no `pg_dump`. The script is piped in on stdin.
  Writing, checking and pruning all happen inside that container, because the dump
  directory is on the Docker host and the agent may not be able to see it.
- **Safety.** The dump is written to a hidden `.partial` file and checked with
  `pg_restore --list`. Only then is it renamed into place, so a failed dump never
  looks like a good one. Any failure fails the build before Teardown, which leaves
  the old deployment running. If no `db` container is running (a first deploy),
  the stage logs that and skips the dump. Files are mode `0600`, since a dump
  holds password hashes and sessions.
- **Retention.** `DUMP_KEEP` in the `Jenkinsfile` (default 30). After each dump,
  every matching file beyond the newest `DUMP_KEEP` is deleted. Pruning sorts by
  file name rather than mtime, and files that don't match the pattern are left
  alone.

Restoring one is [Flashing the database from a dump](#flashing-the-database-from-a-dump).

### Flashing the database from a dump

`store.RestoreDump` replaces the database with the contents of any dump, whatever
schema version wrote it:

1. `pg_restore` the dump, unchanged, into a scratch database `restore_scratch_<ns>`
   on the same server (created from `template0`, dropped afterwards either way).
2. Upgrade the scratch copy in place: apply every migration it has not recorded
   (`store.upgradeSource`; a pre-migration dump takes the idempotent baseline
   first). This is what carries data a migration *moves* — `0002` turns
   `decklists.cube_id` into a cube tag — which a by-name column copy would drop.
   A migration that fails here is not fatal: the copy goes ahead from the last
   version that applied, and the report's `upgrade_error` says so.
3. In **one transaction** on the target: drop and recreate the current schema
   (`public` in prod), apply every migration, then copy each table from the
   scratch database in foreign-key order (`store.Reimport`).
4. Print or return a report: which migrations upgraded the dump, then per table, rows copied and skipped, columns dropped
   and columns blanked.

Copy rules: only columns in both the upgraded dump and the schema are copied. A column the
schema dropped is discarded; one it added gets its default or NULL; generated
columns (`cards.slug`, `decklists.winrate`) are recomputed; `schema_migrations`
is never copied. A renamed column is not detected: it reports as dropped plus
blanked, and its data is lost. Rows move as `COPY` text, so a widened column type
still copies. A table is tried in one `COPY`; if that fails it is retried row by
row under savepoints and the rejected rows (a new `CHECK`, `NOT NULL`, or a foreign
key to a row that was itself skipped) are skipped and listed, not fatal. Any other
error rolls the whole transaction back and the live data is untouched. Nothing
resets sequences, since no table has one; a future `serial` column would need it.

**Never on deploy.** A deploy applies pending migrations in place after the
pre-deploy dump. The drop-and-reimport cycle silently discards data in dropped or
renamed columns, so it runs only when an admin asks for it, from one of two
entry points:

- **CLI**, an ephemeral container that shares the backend image and migrations.
  Stop the live backend first, so it is not serving or running jobs mid-restore:

  ```sh
  docker compose stop backend
  docker compose run --rm backend restore --file /dumps/<file>.dump
  docker compose start backend
  ```

  The image's entrypoint is `/server`, so the subcommand is `restore`, not
  `/server restore`. Dumps written before the backend user could read them are
  root-owned `0600`; add `--user root` for those.
- **Admin API**, in the running backend. `GET /api/admin/db/dumps` lists the
  `*.dump` files in `DUMP_DIR` (default `/dumps`, the dump directory mounted
  read-only), newest first. `POST /api/admin/db/restore` with
  `{"dump": "<file>.dump", "confirm": "replace the database"}` starts the restore
  in the background and returns `202`. While it runs the worker is paused (after
  its job in flight finishes) and every route except `/api/health` answers `503`.
  Afterwards `GET /api/admin/db/restore` returns the state and the report. That
  status lives in memory, not the `jobs` table, which the restore replaces. The
  `sessions` table is restored too, so the admin may need to sign in again.

Both need `pg_restore` (the backend image installs `postgresql16-client`) and a
database role that may `CREATE DATABASE`. Jenkins chowns each new dump to the
backend's uid 10001 so the API can read it. Tests:
`TestReimportIntoLatestSchema` and `TestRestoreDumpFromPreMigrationSchema`, which
skips without `pg_dump` and `pg_restore` on PATH.

### Taking a dump by hand

The pre-deploy dump is automatic, but one can be taken any time — before a risky
restore, or to rehearse a migration elsewhere. On the prod host, from the checkout,
this does what Jenkins' Backup Database stage does (same directory, name pattern,
check and ownership, so retention and the restore API both pick it up):

```sh
docker compose run --rm -T \
  -e DUMP_FILE="mtg_meta_$(date -u +%Y%m%dT%H%M%SZ)_$(git rev-parse --short HEAD).dump" \
  db-dump <<'SH'
umask 077
tmp="/dump/.$DUMP_FILE.partial"
pg_dump -Fc -f "$tmp"
pg_restore --list "$tmp" >/dev/null
chown 10001:10001 "$tmp"
mv "$tmp" "/dump/$DUMP_FILE"
echo "wrote /dump/$DUMP_FILE"
SH
```

To rehearse a restore off prod, copy the file to a machine with the dev database up
and run `cd backend && go run ./cmd/server restore --file <path>.dump` (it uses the
dev `DATABASE_URL` default and needs `pg_restore` on PATH). The report's first line
lists the migrations the dump was upgraded through. Rolling *code* back past a
migration means restoring a dump taken before it with the *old* binary: a new
schema has no columns the old code reads.

## Dependencies

Backend: `go-chi/chi/v5`, `google/uuid`, `jackc/pgx/v5`, `golang.org/x/crypto`,
`golang.org/x/sync`. Nothing else direct.

Frontend: `next`, `react`, `react-dom` at runtime; `typescript` and `@types/*` in
dev. No UI, styling, charting, or data-fetching library.

External: Scryfall only.
