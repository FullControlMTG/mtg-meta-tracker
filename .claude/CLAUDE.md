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
store/          pgx repositories, hand-written SQL. schema.sql lives here.
auth/           argon2id passwords, server-side sessions, middleware.
httpapi/        chi router + handlers. server.go is the route source of truth.
scryfall/       Batched card client resolving exact printings.
ingest/         Cube syncer: pasted list -> resolve -> diff the pool.
decklist/       List parser and card resolver.
images/         Self-hosted card-image cache.
moxfield/       Parses a publicId out of a URL. Display metadata only.
analytics/      Recompute engine (compute.go) + model/ leaf package.
jobs/           Queue, worker, scheduler.
revalidate/     Client for the Next.js revalidation webhook.
```

## Commands

```sh
make db-up      # Postgres on :5432 via docker-compose.dev.yml
make backend    # API on :8080 (go run ./cmd/server)
make frontend   # Next.js on :3000, proxies /api -> :8080
make test       # go vet ./... && go test ./... — no database required
make db-down    # stop Postgres (ARGS=-v also drops the data volume)
make db-dump    # dev db -> db/dump.sql (gitignored)
make db-restore FILE=db/dump.sql
make db-schema  # live schema -> db/schema.generated.sql, for diffing
```

Frontend type-check: `cd frontend && npx tsc --noEmit`.

The backend's default `DATABASE_URL` points at the dev database, so
`make db-up && make backend` runs with no `.env`. A usable app also needs a first
admin — there is no public signup — seeded via the `BOOTSTRAP_ADMIN_*` env vars
(README Quickstart has the exact command).

### What CI actually runs

`Jenkinsfile` stage "Lint & Type-check" builds two throwaway Docker images:

- backend: `go vet ./... && go build ./... && go test ./...`
- frontend: `npx tsc --noEmit`

Then: Prepare Environment, Teardown, Build & Deploy (`docker compose up -d
--build`), Health Check (waits for both containers' Docker healthchecks), Smoke
Test.

**There is no linter.** `frontend/package.json` defines `"lint": "next lint"`, but
the repo has no ESLint config and no ESLint dependency, so the script proves
nothing and CI does not call it. Do not cite it as a passing check.

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

**Branch naming.** Work lands on `main`. The one other branch in history is
`fix-mount-permissions` (kebab-case, descriptive, no prefix).
TODO: confirm whether feature branches and PRs are expected, or whether commits go
directly to `main`.

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

## Three constraints that break things silently

1. **`backend/internal/store/schema.sql` is applied on every boot.** It is
   embedded in the binary and re-run by `store.EnsureSchema` at startup. There is
   no migration tool and no version table. A statement that fails against a
   populated database stops the server from starting. See
   [DESIGN.md](DESIGN.md#schema-as-boot-time-idempotent-sql).
2. **A family of derived statistics was deliberately deleted** (commit `b483fbb`)
   and must not be reintroduced. See [GOALS.md](GOALS.md#non-goals).
3. **Deck color inference exists in two copies** — `scryfall.castColors` in Go and
   `store.castColorCol` in SQL. Change both. See [GLOSSARY.md](GLOSSARY.md).

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
