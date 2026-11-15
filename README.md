# mtg-meta-tracker

Meta analysis tool for your local playgroup. Supply your own cube decklist data
and gain an understanding of what cards are picked the most, and which perform
the best in your favorite format's metagame.

## Stack

- **Backend** — Go (chi + pgx), PostgreSQL
- **Frontend** — Next.js (App Router): static/ISR decklist pages, interactive
  analytics dashboards
- **Data** — cube pool from a pasted decklist, card data + images from Scryfall
- **Auth** — username/password, server sessions; accounts are created by an admin

## Docs

All project documentation lives in [`.claude/`](.claude/CLAUDE.md) — written for
AI coding agents, but the canonical reference for humans too:

- [`.claude/CLAUDE.md`](.claude/CLAUDE.md) — entry point: stack, layout, commands,
  conventions.
- [`.claude/GOALS.md`](.claude/GOALS.md) — scope, current priorities, and what is
  explicitly out of scope (including which derived statistics are deliberately
  *not* computed, and why).
- [`.claude/DESIGN.md`](.claude/DESIGN.md) — architecture, data model, the
  analytics schema, the API surface, and the reasoning behind key decisions.
- [`.claude/IMPLEMENTATION.md`](.claude/IMPLEMENTATION.md) — how the codebase
  evolved; known limitations and tech debt.
- [`.claude/GLOSSARY.md`](.claude/GLOSSARY.md) — MTG and project-specific terms.

## Quickstart (dev)

Prerequisites: Docker, Go 1.22 and Node 20 — the versions CI builds with, pinned
by `backend/go.mod`'s `toolchain` line and `frontend/.nvmrc` (`nvm use`).

**On Windows, run everything from WSL or Git Bash.** The Makefile assumes a POSIX
shell: `make backend` sets env vars with a `VAR=value command` prefix, and
`db-dump`/`db-restore` use `>`/`<` redirects, all of which break under `cmd` and
PowerShell. `.gitattributes` keeps checkouts LF, so `core.autocrlf` doesn't matter.

The backend's built-in defaults already point at the dev database, so no `.env` is
needed to get running — except for the first admin, which must be seeded (there is
no open signup; see [Auth](#auth)).

```sh
make db-up                      # postgres on :5432 (docker-compose.dev.yml)

# terminal 2 — :8080. The BOOTSTRAP_ADMIN_* vars create the first admin, and
# only take effect while the users table is empty.
BOOTSTRAP_ADMIN_USERNAME=admin \
BOOTSTRAP_ADMIN_EMAIL=admin@example.com \
BOOTSTRAP_ADMIN_PASSWORD=devpassword123 \
make backend

# terminal 3 — :3000, proxies /api -> :8080
cd frontend && npm install && cd .. && make frontend
```

Then sign in at http://localhost:3000 as that admin and paste a cube list at
`/admin/cubes`; the pool is built from it in the background by a Scryfall sync job.

```sh
make test                       # go vet ./... && go test ./... (no database needed)
make test-db                    # the same plus the DB tests, against make db-up
cd frontend && npx tsc --noEmit # type-check the frontend, as CI does
```

Note that `docker-compose.yml` is the **production** deployment (Traefik, a NAS bind
mount for Postgres, a Docker volume for the card-image cache, no published DB port)
and is not usable for local dev — `make db-up` uses `docker-compose.dev.yml` instead.
Jenkins dumps the prod database to `/pwspool/software/mtg-meta-tracker/dump/` before
every deploy and keeps the newest 30 (see `.claude/DESIGN.md`, "Deployment"). To
flash the database from one, whatever schema wrote it, run
`docker compose run --rm backend restore --file /dumps/<file>.dump` with the backend
stopped, or use the admin `POST /api/admin/db/restore` API (`.claude/DESIGN.md`,
"Flashing the database from a dump").

## Auth

Username/password only, with **no public signup**: the first admin comes from the
`BOOTSTRAP_ADMIN_*` env vars above, and creates everyone else from `/admin/users`
(`POST /api/admin/users`), handing over an initial password. Users change their own
password under `/settings`; an admin can reset anyone's without knowing the old one.

## Conventions

House rules, so the codebase stays one thing rather than five:

- **No ORM.** Hand-written SQL in `backend/internal/store/*.go`.
- **No Tailwind, no CSS modules, no CSS-in-JS.** One global stylesheet,
  `frontend/app/globals.css`, built on CSS custom properties (with a
  `prefers-color-scheme: dark` block) plus semantic classes; everything else is
  inline `style={{…}}`.
- **No chart library.** Every chart — `ColorWinrateChart`, `RadarChart`,
  `ColorTrendChart` — is hand-rolled SVG.
- **Routes live in one place**: `backend/internal/httpapi/server.go` is the source
  of truth for the API surface.
- Frontend components are flat in `frontend/components/`, PascalCase, named
  exports.

## Database

The schema is a series of numbered migrations in
[`backend/internal/store/migrations/`](backend/internal/store/migrations/)
(`NNNN_description.sql`), embedded in the Go binary. On startup the backend applies
each one not yet recorded in the `schema_migrations` table, in order, each in its
own transaction. A failing migration rolls back and **stops the server from
starting**. `0001_baseline.sql` is the schema as it stood before migrations.

To change the schema, add the next-numbered file. Never edit one that has shipped:
databases that already applied it will not run it again. A migration runs once,
so it can use plain `ALTER TABLE`, `DROP COLUMN` or a one-shot backfill, but it
runs inside a transaction (no `CREATE INDEX CONCURRENTLY`) and against real data
(backfill before adding `NOT NULL` or a `CHECK`).

`db/` is a scratch directory for gitignored dumps — the schema is *not* there.

```sh
make db-dump                       # back up schema + data -> db/dump.sql (gitignored)
make db-restore FILE=db/dump.sql   # import a dump into the running db
make db-schema                     # dump the live schema for diffing (gitignored)
make db-down ARGS=-v               # stop postgres and drop its data volume
```
