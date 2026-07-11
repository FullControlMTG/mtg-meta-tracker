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
  browser  ◀──────▶ │  • /decklists/[id]  static, ISR revalidate │
                    │  • /users/[name]    static, ISR revalidate │
                    │  • /analytics       client, interactive    │
                    │  • /decks/new       live color inference    │
                    └───────────────┬──────────────┬────────────┘
                        rewrites /api│              │POST /api/revalidate
                                     ▼              ▲ (on-demand render)
                    ┌──────────────────────────────┴────────────┐
                    │              Go backend (chi)               │
                    │  httpapi ─ appctx (Caller) ─ auth           │
                    │  store (pgx) ── PostgreSQL                  │
                    │  scryfall client   moxfield client          │
                    │  jobs worker ── analytics engine            │
                    └───────────────┬────────────────────────────┘
                                    ▼
                         Scryfall API   Moxfield API
```

- **Frontend / backend split.** Next.js owns rendering + UX. Go owns data,
  auth, external integrations, and analytics. They talk JSON over `/api`, proxied
  by a Next.js rewrite so the session cookie stays same-site (no CORS headaches).
- **Render-on-update.** When a deck, record, or the cube pool changes, the Go
  backend (after recompute) calls a Next.js `/api/revalidate` route handler with
  a shared secret, which runs `revalidatePath` for the affected static pages.
  Decklist and user pages therefore render once and only re-render on real change.
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
    auth/                password, google oauth, sessions, middleware
    httpapi/             chi router + handlers
    scryfall/            batched card client
    moxfield/            cube-list adapter
    analytics/           recompute engine + queries
    jobs/                queue + worker
db/schema.sql            full schema, auto-applied by Postgres on first init
frontend/                Next.js app
docs/                    this file + ROADMAP.md
```

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
| `oauth_accounts` | Google links | `user_id, provider, provider_account_id` |
| `sessions` | server-side sessions (revocable) | `id (token), user_id, expires_at` |
| `cubes` | a card pool = one Moxfield list | `id, name, moxfield_public_id, last_synced_at` |
| `cards` | Scryfall cache | `scryfall_id (pk), oracle_id, name, cmc, type_line, colors, color_identity, rarity, image_* , raw jsonb` |
| `cube_cards` | pool membership + history | `cube_id, card_id, added_at, removed_at (nullable), is_active` |
| `decklists` | deck + metadata + record | see below |
| `decklist_cards` | normalized deck contents | `decklist_id, card_id, card_name, quantity, is_resolved, board` |

### `decklists`

Metadata + the list + the record all live together (record is nullable and
added after the fact), per requirement.

```
id                uuid pk
cube_id           fk
user_id           fk           -- uploader; owner + admins get U/D
name              text
description       text null
color_identity    smallint     -- inferred bitset
archetype         text null    -- enum: aggro | control | midrange | tempo | combo
source_url        text null    -- moxfield link
decklist_raw      text         -- raw "1 Lightning Bolt\n…" (fits varchar/text)
card_count        int
status            text         -- draft | active | archived
-- record (nullable, added later; updating these triggers recompute)
games_played      int  default 0
wins              int  default 0
losses            int  default 0
draws             int  default 0
placement         int  null    -- finish in an event, if applicable
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

Three facets in one table so the dashboard can slice color performance three ways:

```
color_stats(run_id, facet, facet_key,
            deck_count, games, wins, losses, draws,
            winrate, avg_placement)
```
- `facet = 'exact_identity'` → `facet_key` = the 0–31 bitset (WUBRG combos).
- `facet = 'single_color'`  → `facet_key` = one color bit (decks *containing* W…).
- `facet = 'color_count'`   → `facet_key` = 0–5 (mono/two/three-color…).

Answers "do blue decks win more?", "is two-color better than five-color?", and
per-combo winrates.

### 4.2 Card stats — `card_stats`

Per pool card, the "picked most" + "performs best" signals from the README:

```
card_stats(run_id, card_id,
           deck_count,            -- how many decks run it
           inclusion_rate,        -- deck_count / total_decks  → popularity
           games, wins, losses, draws,
           winrate,               -- raw winrate of decks running it
           winrate_shrunk,        -- Bayesian-smoothed (see §4.5)
           winrate_lift,          -- winrate_shrunk − global_winrate → power signal
           wilson_lower)          -- ranking-safe lower bound
```
- **Popularity** = `inclusion_rate`.
- **Performance** = `winrate_lift` (how much better decks with this card do vs
  the field) with `wilson_lower` for honest ranking on small samples.

### 4.3 Co-occurrence / "played with" — `card_pair_stats`

Powers "cards played with XYZ" suggestions via association-rule metrics:

```
card_pair_stats(run_id, card_a_id, card_b_id,
                co_count,            -- decks with both
                support,            -- co_count / total_decks
                confidence_ab,      -- co_count / count(A): P(B | A)
                lift,               -- support / (support_a * support_b)
                pair_winrate)
```
Stored only for pairs with `co_count >= 2` to bound the n² blow-up. `lift > 1`
means the pair appears together more than chance — the ranking signal for
suggestions; `confidence_ab` gives the natural-language "played alongside X in
Y% of its decks".

### 4.4 Meta overview + deck-property correlations

```
meta_snapshot(run_id, total_decks, total_games, overall_winrate,
              avg_cmc, avg_color_count, mono_share, multi_share, …)

deck_metric_stats(run_id, metric, bucket,       -- e.g. metric='avg_cmc'
                  deck_count, winrate)          -- winrate by CMC/creature-count bucket
```
Lets the dashboard chart "does a lower curve win?" and headline meta numbers.

### 4.5 Making the analysis *deeper* (and statistically honest)

Small-playgroup data is noisy; raw winrates mislead. The engine applies:

1. **Bayesian shrinkage** of every card/color winrate toward the global winrate
   with a pseudo-count *k* (Beta-Binomial): `(wins + k·μ)/(games + k)`. A card
   with a 100% winrate over 2 games won't top the chart.
2. **Wilson score lower bound** for ranking "best cards / colors" — rewards both
   high winrate *and* sample size.
3. **Lift, not raw co-occurrence**, for suggestions — surfaces genuinely
   associated cards, not just staples that appear everywhere.
4. **Card recommendation** (future endpoint): given a partial list + target color
   identity, score candidate cards by
   `α·inclusion_rate + β·winrate_lift + γ·Σ lift(candidate, chosen_i)` — blends
   "commonly played", "performs well", and "synergizes with your picks".
5. **Synergy graph**: `card_pair_stats.lift` is an edge list → a network view /
   "synergy explorer" on the analytics page.
6. **Time-series**: because runs are retained, we can chart meta evolution
   (color share, card inclusion) across runs.
7. **Archetype clustering** (future): vectorize decks by card membership and
   cluster (Jaccard/k-means) to auto-discover archetypes beyond the free-text tag.

**Head-to-head is intentionally out of scope** given the aggregate-record model;
if per-game matchups are wanted later, add a `matches` table and a matchup facet.

---

## 5. External integrations

### Scryfall (card data + images)
- Resolve the cube's card names in **batches via `POST /cards/collection`**
  (≤75 identifiers per request). Set a descriptive `User-Agent` + `Accept:
  application/json`, sleep ~75–100 ms between requests, exponential backoff on 429.
- Cache full payloads in `cards.raw` (jsonb) plus extracted columns; refresh on a
  schedule. Images used: `art_crop` (overlaid deck view) and `normal` (detail).

### Moxfield (cube list source)
- Parse the `publicId` from the deck URL and fetch the list via Moxfield's
  (unofficial) deck API behind a small **adapter interface**, so if access breaks
  we can drop in manual paste / CSV import without touching callers.
- Periodic sync diffs the fetched list against `cube_cards`: new names →
  Scryfall resolve → insert; missing names → set `removed_at`, `is_active=false`
  (soft, to preserve historical decklist references).

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
- `jobs(id, type, payload jsonb, status, scheduled_at, attempts, last_error)`;
  worker goroutine polls + dedups so a burst of edits = one recompute.
- Scheduled jobs: daily Moxfield cube sync + Scryfall refresh.

---

## 7. API surface (representative)

Public (accepts anonymous Caller):
`GET /api/cube`, `/api/decklists`, `/api/decklists/:id`, `/api/users/:name`,
`/api/analytics/overview|colors|cards|pairs`, `/api/cards/:id`.

Auth: `POST /api/auth/register|login|logout`, `GET /api/auth/google[/callback]`,
`GET /api/auth/me`.

Mutations (require Authenticated + ownership/role):
`POST/PATCH/DELETE /api/users/:id`, `POST/PATCH/DELETE /api/decklists/:id`,
`PATCH /api/decklists/:id/record`, `POST /api/decklists/infer-colors`.

Admin/ops: `POST /api/admin/cube/sync`, `POST /api/admin/analytics/recompute`.

---

## 8. Color-identity inference

`POST /api/decklists/infer-colors` (and on save): parse the raw list → resolve
names to `cards` → OR together each card's `color_identity` bitset → deck
identity. The `decks/new` page calls this live as the user pastes.

The inference sits behind a strategy interface so it can grow: today a simple OR;
later, splash detection (weight by pip counts), ignoring off-color hybrid,
land-vs-spell handling, etc.

---

## 9. Key frontend pages

- `/` — headline meta dashboard (from `meta_snapshot`).
- `/analytics` — dense, interactive: color winrate charts, sortable card table
  (popularity vs lift vs Wilson), synergy explorer, meta trends. Charts built
  with the `dataviz` guidance.
- `/decklists` + `/decklists/[id]` — static/ISR. Detail page is the compact
  **overlaid card fan**: Scryfall images stacked with ~90% overlap (only the top
  ~10% name line peeks) using CSS negative margins, plus record + card stats.
- `/users/[name]` — bio + dense decklist list with per-deck stats.
- `/decks/new` — paste list, live color inference, record entry.
