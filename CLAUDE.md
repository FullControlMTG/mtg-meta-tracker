# mtg-meta-tracker

Meta-analysis tool for a local MTG cube playgroup. Go (chi + pgx) API + Postgres
behind a Next.js 14 App Router frontend. See [`docs/DESIGN.md`](docs/DESIGN.md)
for architecture and [`docs/ROADMAP.md`](docs/ROADMAP.md) for what's left.

This file covers the things that are easy to get *wrong* here. It is not a tour
of the repo — the README is.

## Commands

```sh
make db-up      # postgres on :5432 (docker-compose.dev.yml)
make backend    # API on :8080
make frontend   # Next.js on :3000, proxies /api -> :8080
make test       # go vet ./... && go test ./... — no database needed
```

Frontend type-checking is `cd frontend && npx tsc --noEmit`; that is what CI
runs. `npm run lint` is **unwired** — the script exists but there is no ESLint
config or dependency, so it proves nothing.

`docker-compose.yml` is the **production** deployment (Traefik, a NAS bind mount
for Postgres, no published DB port) and cannot be used locally. Dev always goes
through the Makefile, which passes `-f docker-compose.dev.yml`.

## The schema is applied on every boot, so it must stay idempotent

`backend/internal/store/schema.sql` is embedded in the Go binary and re-applied
on **every** backend startup by `store.EnsureSchema`. There is no migration tool,
no migration directory, and no version table.

This means a statement that fails on an already-populated database **stops the
server from starting**. So:

- New tables/indexes: `CREATE TABLE/INDEX IF NOT EXISTS`.
- New columns: `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`, in the
  `Idempotent migrations for existing databases` section at the bottom of the
  file — *not* in the `CREATE TABLE` block, which is a no-op once the table
  exists.
- New CHECK constraints: normalize the existing rows first, then
  `DROP CONSTRAINT IF EXISTS` before `ADD CONSTRAINT` (the archetype enum at the
  bottom of the file is the worked example).

`db/` is a scratch directory for gitignored dumps. There is no `db/schema.sql`.

## Do not reintroduce the removed statistics

Bayesian-shrunk winrates, lift, Wilson lower bounds, and pair
support/confidence/lift were all computed once and were **deliberately deleted**
(commit `b483fbb`). Do not add them back, and do not reach for them when asked to
make the analytics "more rigorous".

The reason is in the input, not the math: records are stored **per deck**, so a
card's "games" are its *deck's* games attributed wholesale to all ~40 cards. That
attribution is the weak link, and no amount of statistical machinery downstream
repairs it — it just dresses one soft number up as three. The fix, if we want a
defensible power signal, is per-game results. `docs/DESIGN.md` §4.5 has the full
argument; read it before touching the analytics engine.

Head-to-head / matchup stats are out of scope for the same reason.

## Colors

Colors are a 5-bit bitset in a `SMALLINT`: `W=1 U=2 B=4 R=8 G=16`, colorless `0`.
See `backend/internal/domain/color.go`.

A deck's colors come from what it **casts** — the colors in the casting costs of
its nonland cards (Scryfall `colors`) — and never from `color_identity`. Color
identity counts mana a card *produces*, which is how a Selesnya deck running a
Mox Sapphire used to come out blue.

A color on under 10% of a deck's nonlands (`domain.SplashThreshold`) is a
**splash**: stored separately in `decklists.splash_colors`, kept out of
`color_identity`, and excluded from every color analytic except the
`splash_color` facet.

Colors are inferred at save time, so the rule can drift from what's stored.
`store.RecomputeDeckColors` re-derives every deck in a cube and the analytics job
runs it before aggregating — so a rule change converges on the next run without a
data migration. Change the rule, don't write a backfill.

## Conventions

- **No ORM.** Hand-written SQL in `backend/internal/store/*.go`.
- **No Tailwind, no CSS modules, no CSS-in-JS.** One global stylesheet,
  `frontend/app/globals.css`, built on CSS custom properties (with a
  `prefers-color-scheme: dark` block) plus semantic classes; everything else is
  inline `style={{…}}`.
- **No chart library.** `ColorWinrateChart` and `RadarChart` are hand-rolled SVG.
- **Routes live in one place**: `backend/internal/httpapi/server.go`. It is the
  source of truth for the API surface — check it before documenting a route.
- Frontend components are flat in `frontend/components/`, PascalCase, named
  exports.

## Cube ingestion does not use the Moxfield API

Moxfield blocks us. A cube's pool is a **raw pasted decklist** stored in
`cubes.card_list` (the source of truth), fingerprinted by `content_hash` so an
unchanged list skips re-resolution. `backend/internal/moxfield` survives only to
parse a `publicId` out of a URL for display.

Names are resolved to exact printings via Scryfall's batched
`POST /cards/collection`; anything unresolvable is surfaced in
`cube_sync_progress.unresolved` rather than silently shrinking the pool. Card
images are self-hosted (`backend/internal/images`, `/data/images` volume in prod),
not hotlinked.

## Auth

Username/password (argon2id) with opaque server-side sessions. **No public
signup**: the first admin comes from the `BOOTSTRAP_ADMIN_*` env vars, which only
take effect while the `users` table is empty; that admin creates everyone else.
Google OAuth is *not implemented* — the `GOOGLE_*` config and the
`oauth_accounts` table exist, but there is no route.

Every request carries an `appctx.Caller` (public vs authenticated, plus role).
Read endpoints accept a public caller; mutations require auth plus an
ownership/role check.
