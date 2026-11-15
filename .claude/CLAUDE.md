# mtg-meta-tracker — agent entry point

## Purpose

Meta-analysis tool for a single local Magic: The Gathering cube playgroup. An
admin pastes a cube's card list; players upload the decks they build from that
pool and record win/loss results. The backend resolves every card name against
Scryfall, infers each deck's colors from its casting costs, and recomputes
aggregate snapshots — color share, card popularity, card co-occurrence, deck
metrics — which the frontend renders as a metagame dashboard. It is deployed for
one playgroup at `cube.fullcontrolmtg.com`, not as a multi-tenant product.

## Tech stack

| Layer | Technology |
|---|---|
| Backend | Go 1.22 — chi v5 (router), pgx v5 (Postgres driver), `golang.org/x/crypto` (argon2id) |
| Database | PostgreSQL 16 (`postgres:16-alpine`) |
| Frontend | Next.js 14.2.5 (App Router), React 18.3.1, TypeScript 5.5.3 |
| CI/CD | Jenkins (`Jenkinsfile`), Docker Compose, Traefik reverse proxy |
| External API | Scryfall (`POST /cards/collection`) |

Primary languages: Go, TypeScript, SQL.

Package managers: Go modules (`backend/go.mod`); npm (`frontend/package-lock.json`
is committed — npm, not yarn or pnpm).

There is no ORM, no CSS framework, and no chart library. See "Conventions".

## Directory layout

```
backend/        Go API. cmd/server wires it; internal/ holds every package.
frontend/       Next.js app: app/ (routes), components/ (flat), lib/ (helpers).
db/             Scratch directory for gitignored dumps. Contains no schema.
.claude/        These knowledge files.
```

Inside `backend/internal/`:

```
appctx/         Caller context: public vs authenticated, plus role.
config/         Env-var config loading.
domain/         Core types: the WUBRG color bitset, enums, deck models.
store/          pgx repositories, hand-written SQL. migrations/ lives here.
auth/           argon2id passwords, server-side sessions, middleware.
httpapi/        chi router + handlers. server.go is the route source of truth.
scryfall/       Batched card client resolving exact printings.
ingest/         Cube syncer: pasted list -> resolve -> diff the pool.
decklist/       List parser and card resolver.
images/         Self-hosted card-image cache.
moxfield/       Parses a publicId out of a URL. Display metadata only.
analytics/      Recompute engine (compute.go) + model/ leaf package.
jobs/           Queue, worker, scheduler.
dbrestore/      Admin restore API's runner (dump listing, maintenance).
revalidate/     Client for the Next.js revalidation webhook.
testdb/         Per-test Postgres schema for DB tests (TEST_DATABASE_URL).
```

## Commands

```sh
make db-up      # Postgres on :5432 via docker-compose.dev.yml
make backend    # API on :8080 (go run ./cmd/server)
make frontend   # Next.js on :3000, proxies /api -> :8080
make test       # go vet ./... && go test ./... — no database required
                # DB tests skip unless TEST_DATABASE_URL is set; each runs in
                # its own throwaway Postgres schema, so the dev db is safe to use
make test-db    # go test ./... with TEST_DATABASE_URL at the make db-up database
make db-down    # stop Postgres (ARGS=-v also drops the data volume)
make db-dump    # dev db -> db/dump.sql (gitignored)
make db-restore FILE=db/dump.sql
make db-schema  # live schema -> db/schema.generated.sql, for diffing
```

Frontend type-check: `cd frontend && npx tsc --noEmit`.

Toolchain: Go 1.22 and Node 20, matching CI's `golang:1.22-alpine` and
`node:20-alpine`. They are pinned in `backend/go.mod` (`toolchain`),
`frontend/.nvmrc`, and `frontend/package.json` (`engines`); bump all of them with
the Dockerfiles and the `Jenkinsfile`. The Makefile needs a POSIX shell, so
Windows means WSL or Git Bash, not `cmd` or PowerShell. `.gitattributes` forces LF.

The backend's default `DATABASE_URL` points at the dev database, so
`make db-up && make backend` runs with no `.env`. A usable app also needs a first
admin — there is no public signup — seeded via the `BOOTSTRAP_ADMIN_*` env vars
(README Quickstart has the exact command).

### What CI actually runs

`Jenkinsfile` stage "Lint & Type-check" builds two throwaway Docker images:

- backend: `docker build --target test backend` — the `test` stage of
  `backend/Dockerfile`, `go vet ./... && go test ./...` on top of the same build
  stage the deployed image uses. Change the backend build there, not in CI.
- frontend: `docker build --target typecheck .` — the `typecheck` stage of the
  root `Dockerfile`, `npx tsc --noEmit` on the same `deps` stage (`npm ci`) the
  prod image builds from. `npm ci` installs exactly `package-lock.json`, so commit
  lockfile changes with any `package.json` change or both builds fail.

Both Dockerfiles use BuildKit cache mounts (Go's build cache, npm's cache,
`.next/cache`), so a source change recompiles only what changed. Jenkins sets
`DOCKER_BUILDKIT=1` and Preflight requires `docker buildx`. Don't put a
`--mount` back behind the legacy builder, and don't put the Go module download
in a cache mount: the integration run has no internet, so the modules must be in
the image.

Then Integration Tests, on every branch: a throwaway `postgres:16-alpine` on a
private Docker network, and `go test -count=1 ./...` run in the same `test` image
against it with
`TEST_DATABASE_URL` set, so the DB tests run too; the container, network and image
are removed whatever the result. A failing test fails the build, which on `main` is
before anything is torn down.

DB tests are scoped to the critical paths: migrations (empty database to latest,
pre-migration schema upgraded), the cube access boundary, and decklist create →
record → analytics run. New ones use `testdb.Pool`. Nothing in CI calls Scryfall,
and there are no frontend E2E tests.

Then Prepare Environment. Only on `main`, the `Deploy` group follows: Build
Images (`docker compose build`, while prod still serves), Backup Database
(`pg_dump -Fc` to `/pwspool/software/mtg-meta-tracker/dump/`; a failure aborts
the build before anything is torn down), Teardown, Start (`up -d --no-build`),
Health Check (waits for both containers' Docker healthchecks), Smoke Test. See
[DESIGN.md](DESIGN.md#deployment).

Restoring prod from one of those dumps is `docker compose run --rm backend restore
--file /dumps/<file>.dump` (backend stopped) or `POST /api/admin/db/restore`. It
rebuilds the schema and reimports best-effort; it never runs on deploy. See
[DESIGN.md](DESIGN.md#flashing-the-database-from-a-dump).

**There is no linter.** There is no ESLint config, dependency or `lint` script
(the old no-op `next lint` script was removed). `tsc --noEmit` is the only
frontend check.

## Conventions an agent must follow

**Formatting.** Go: standard `gofmt`. TypeScript/TSX: 2-space indent, double
quotes, semicolons, trailing commas, ~90-column lines — Prettier defaults, though
no Prettier config is committed.

**Comments** explain *why*, not *what*: the constraint, the bug that motivated the
line, the thing a reader would otherwise "simplify" away. Match that register.

**Commit style.** Imperative, lower-case-after-first-word subject lines under ~72
characters ("Infer deck colors from casting cost, and split off splashes"). Bodies
are prose paragraphs explaining the motivation and the trade-off, wrapped at ~76
columns. No Conventional Commits prefixes, no ticket IDs. AI-assisted commits
carry a `Co-Authored-By:` trailer.

**Branches and PRs.** Work lands on `main` through pull requests: branch from
`main`, open the PR against `main`, and don't stack on another open PR. Only
`main` deploys. Human branches are kebab-case and descriptive with no prefix
(`fix-mount-permissions`, `unit-and-db-integration-tests`); agent branches use
`agent/<agent>/<id>`.

**Code rules** — these are choices, not accidents:

- No ORM. Hand-written SQL in `backend/internal/store/*.go`.
- No Tailwind, no CSS modules, no CSS-in-JS. One stylesheet,
  `frontend/app/globals.css`, of CSS custom properties (with a
  `prefers-color-scheme: dark` block) and semantic classes; everything else is
  inline `style={{…}}`.
- No chart library. Every chart is hand-rolled SVG.
- Routes live only in `backend/internal/httpapi/server.go`.
- Frontend components are flat in `frontend/components/`, PascalCase, named
  exports.
- One mobile breakpoint: `max-width: 640px`, at the bottom of `globals.css`.

## Constraints that break things silently

1. **Schema changes are append-only migrations.** Files in
   `backend/internal/store/migrations/` are applied once each, in order, by
   `store.Migrate` at startup, and recorded in `schema_migrations`. Never edit a
   shipped file; add the next number. A failing file stops the server from
   starting. See [DESIGN.md](DESIGN.md#schema-as-versioned-migrations).
2. **A family of derived statistics was deliberately deleted** (commit `b483fbb`)
   and must not be reintroduced. See [GOALS.md](GOALS.md#non-goals).
3. **Card colors are game rules and live in one place** — `domain.PrintedColors`
   (`CardColors` for sorting/grouping, `CastColors` for deck inference). Ingest and
   every SQL read (`store.printedColorsCol` projects the raw Scryfall fields) feed
   it; don't reimplement either rule in SQL or TypeScript. See
   [GLOSSARY.md](GLOSSARY.md).
4. **Decks are not owned by cubes.** There is no `decklists.cube_id`: a cube's
   decks are the ones whose `tags` hold its `deck_tag`. Filter with
   `store.deckInCube`, which also treats a deck in several cubes correctly. See
   [DESIGN.md](DESIGN.md#decks-are-the-unit-cubes-select-them-by-tag).

## Other files here

- [GOALS.md](GOALS.md) — scope, priorities, explicit non-goals.
- [DESIGN.md](DESIGN.md) — architecture, components, design decisions and
  trade-offs.
- [IMPLEMENTATION.md](IMPLEMENTATION.md) — how the codebase got here; known
  limitations and tech debt.
- [GLOSSARY.md](GLOSSARY.md) — domain and project-specific terms.

`README.md` at the repository root is the human-facing entry point: what the
project is, how to run it, and the same conventions in brief. It and this folder
are the only prose documentation — a `docs/` folder existed and was folded in
here.

Where any of this disagrees with the code, the code is right and the file is
stale. Fix it in the same change.
