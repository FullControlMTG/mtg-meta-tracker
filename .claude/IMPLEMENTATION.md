# Implementation history

Reconstructed from `git log` on `main`. Commit hashes are the record. This file
absorbed the former `docs/ROADMAP.md`, which has been removed.

## Build phases, as delivered

The project was built to a phase plan. Phases 0–5 are complete; Phase 6 is
partly done. What shipped in each:

- **Phase 0 — Foundation.** Design doc and analytics schema; the Postgres schema
  embedded in the binary; repo scaffold (compose files, Makefile, `.env.example`);
  Go module and `main.go` wiring, `appctx`, `config`, `domain/color`; the Next.js
  skeleton and its `/api` rewrite.
- **Phase 1 — Backend core and auth.** pgx repositories for users, sessions,
  cubes, cards, and jobs; argon2id passwords, sessions, and `appctx` middleware;
  admin-created accounts with first-admin bootstrap; user CRUD with role and
  ownership checks. Google OAuth was planned here and never wired.
- **Phase 2 — Cube ingestion.** Rate-limited batched Scryfall client resolving
  exact printings; admin cube CRUD plus a sync trigger; the jobs worker and its
  `sync_cube` handler diffing `cube_cards`; the move to pasted lists with
  `content_hash`; unresolved-name surfacing; the self-hosted image cache; the
  periodic re-sync scheduler.
- **Phase 3 — Decklists.** Decklist CRUD and list parsing into `decklist_cards`;
  color inference with a cube-pool-then-Scryfall resolver strategy; the record
  endpoint enqueueing a recompute; casting-cost inference with splashes split off;
  the archetype enum; removal of draws and placement.
- **Phase 4 — Analytics engine.** The polling, deduplicating worker; the recompute
  engine writing color/card/pair/meta/metric snapshots; analytics read endpoints;
  the Next.js revalidation webhook; scoping every stat to a cube; removal of the
  derived shrinkage/Wilson/pair-lift statistics.
- **Phase 5 — Frontend.** The analytics dashboard (hand-rolled SVG charts,
  sortable tables, MTG palette); deck detail with the overlaid card fan; user
  pages and deck creation with live inference; auth pages and admin management;
  card and cube pool pages.
- **Phase 6 — Polish, in progress.** Go unit tests and the Jenkins pipeline
  shipped. The remaining items are listed in [GOALS.md](GOALS.md#current-priorities).

## Milestones

**Scaffold and deployment (`8c18870` → `48c9524`).** Initial commit, then
`c5d3c44` laid down the design doc, schema, auth, and cube ingestion in one go.
Deployment came early: `99570ad` dockerized the app behind Traefik, `268b141`
added the Jenkins pipeline, `34c2355` pointed production at a hostname, and
several commits fixed CI mechanics (`370ad38` shipping source as build context
instead of a bind mount, `6b6a9c8` renaming credentials, `aa8a390` baking
`BACKEND_ORIGIN` into the frontend build).

**Schema delivery model, twice revised.** `efcd26a` replaced a migration
container with a committed schema dump; `dcdda9a` then moved to applying the
schema idempotently from the Go binary on every startup, which is the current
model.

**Features (`b232b07` → `6d70fd8`).** `b232b07` implemented roadmap phases 3–5 —
decklists, the analytics engine, and the frontend — in a single commit. `6d70fd8`
followed with admin cube management, periodic sync, deck records, and a login
session fix. `b4d8027` added the public read-only cubes browser.

**Images and ingestion hardening (`894883d` → `10a2f83`).** Card images moved to a
self-hosted download-on-miss cache (`894883d`), gained throttling and retries
(`4ce05e8`), prefetch on sync (`a185718`), and a Docker volume replacing an
init container (`226efd3`), with permission fixes along the way (`2f1770f`,
`be44537`). `372da2f` handled DFC and flavor-name resolution. The pivotal change
was `c229182`: cube pools moved from the Moxfield API to pasted decklists, because
Moxfield blocks the app. `7a21be0` and `10a2f83` then surfaced names that failed
to resolve instead of letting them silently shrink the pool.

**Data-model tightening.** `8cbf33e` constrained deck archetype to a fixed enum.
`31d9f24` dropped draws and placement from the record. `1cc213d` scoped every stat
to a cube and added card pages.

**Statistics removal (`b483fbb`).** Lift, Wilson bounds, Bayesian shrinkage, and
association-rule pair metrics were deleted and the pairs table rebuilt on plain
co-occurrence. The reasoning is in [GOALS.md](GOALS.md#non-goals) — this is the single most important commit to know
about before touching analytics.

**Color inference rewrite (`b1cf33a` → `852b2a5`).** `b1cf33a` grouped cards by
derived colors rather than raw color identity. `3fe29c9` changed deck colors to
derive from casting costs and split sub-threshold colors off as splashes.
`852b2a5` fixed multi-faced cards: only faces with a mana cost contribute, so a
transform card's back no longer colors the deck.

**Frontend rework (`4546819` → `f83d567`).** `4546819` reworked the analytics view
with hover charts, sortable cards, and the MTG palette; `07015d8` unified both
sortable tables onto one sort primitive; `4926381`, `0287516`, and `5750491`
built the card-fan rendering used by both cube and deck pages; `f83d567` added the
single mobile breakpoint and a card search.

**Documentation realignment (`baa72dd`).** `docs/DESIGN.md` and `docs/ROADMAP.md`
had drifted — they still described a `db/schema.sql` migration model and Moxfield
API fetching, and advertised four routes that do not exist. This commit corrected
them and added the original root `CLAUDE.md`.

## Uncommitted work in progress

At the time of writing, the working tree contains a substantial unstaged change
set. Verify against `git status` before relying on this list.

- `played_at` promoted from a record field to a deck field: `NOT NULL`,
  defaulting to today, owner-editable, decoupled from the record PATCH; a sortable
  Date column in the shared deck table; `ListDecklists` now orders by it.
- `APP_TIMEZONE` config, `GET /api/today`, and `time/tzdata` compiled in.
- `color_trend_stats` table, its aggregation, `GET /api/analytics/color-trend`,
  and a hand-rolled `ColorTrendChart` (100% stacked area).
- `cube_cards.quantity` threaded through ingest, store, API, and the pool page, so
  a cube running many copies of a card badges them; quantity folded into the
  content hash and `resolverVersion` bumped to 3.
- `CardBrowser` search made optional (on for cube pools, off for decklists).
- `frontend/tsconfig.tsbuildinfo` removed from version control.
- The root `CLAUDE.md` replaced by this `.claude/` folder.

## Known limitations and tech debt

**Per-deck records are the ceiling on analytics.** A card inherits its deck's
entire record, so every card-level winrate rests on that attribution. This is the
root cause behind the removed statistics and the top roadmap item.

**No database tests.** `store/` and `httpapi/` are untested. Every existing Go
test is a pure unit needing no database, which is why `make test` requires no
Postgres — and why SQL changes are unverified until run against a real database.
Current test files: `analytics` (10), `scryfall` (9), `decklist` (6), `domain` (4),
`ingest` (4), `auth` (2), `images` (2), `httpapi` (2), `moxfield` (1).

**No frontend tests.** None exist. `npx tsc --noEmit` is the only frontend check.

**No linter.** `frontend/package.json` has a `lint` script but no ESLint config or
dependency. CI does not run it.

**Analytics runs are never pruned.** Every recompute inserts a full set of stat
rows into `analytics_runs` and its child tables, and nothing deletes them.
`card_pair_stats` grows roughly with the square of the pool, so it dominates.
TODO: decide a retention policy.

**Google OAuth is half-present.** The `oauth_accounts` table and `GOOGLE_*`
config exist; the Jenkinsfile even injects the credentials. No route implements
it. Do not document it as a feature.

**Two copies of the color rule.** `scryfall.castColors` (Go, at ingest) and
`store.castColorCol` (SQL, for recompute) implement the same logic separately and
must be changed together. Nothing enforces that.

**Deployment is single-host and manual-ish.** Jenkins runs `docker compose up -d
--build` on one machine, with Postgres on a NAS bind mount
(`/pwspool/software/mtg-meta-tracker/postgres`). There is no staging environment.
TODO: confirm whether a backup schedule exists for that path, or whether backups
are ad hoc.

## Areas under active change

Deck dates, the color-share time series, and cube quantities — all in the
uncommitted set above. Expect `decklists`, `cube_cards`, and the analytics engine
to be in flux.
