# mtg-meta-tracker

Meta analysis tool for your local playgroup. Supply your own cube decklist data
and gain an understanding of what cards are picked the most, and which perform
the best in your favorite format's metagame.

## Stack

- **Backend** — Go (chi + pgx), PostgreSQL
- **Frontend** — Next.js (App Router): static/ISR decklist pages, interactive
  analytics dashboards
- **Data** — cube pool from a pasted decklist, card data + images from Scryfall
- **Auth** — email/password, server sessions; onboarding is admin-invite-only

## Docs

- [`docs/DESIGN.md`](docs/DESIGN.md) — architecture, data model, and the
  analytics schema (color/card/pair stats, shrinkage + Wilson ranking, synergy).
- [`docs/ROADMAP.md`](docs/ROADMAP.md) — phased build plan.

## Quickstart (dev)

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

Note that `docker-compose.yml` is the **production** deployment (Traefik, a NAS bind
mount for Postgres, a Docker volume for the card-image cache, no published DB port)
and is not usable for local dev — `make db-up` uses `docker-compose.dev.yml` instead.

## Auth

Email/password only, with **no public signup**: the first admin comes from the
`BOOTSTRAP_ADMIN_*` env vars above, and everyone else joins via an admin-issued
invite (`POST /api/admin/invites`).

## Database

The schema lives in
[`backend/internal/store/schema.sql`](backend/internal/store/schema.sql). It is
embedded in the Go binary and applied on **every** backend startup, so there is no
migration step or tooling — but it means the file must stay idempotent: use
`CREATE TABLE/INDEX IF NOT EXISTS`, and add columns to existing databases with an
`ALTER TABLE ... ADD COLUMN IF NOT EXISTS` in the migrations section at the bottom.

```sh
make db-dump                       # back up schema + data -> db/dump.sql (gitignored)
make db-restore FILE=db/dump.sql   # import a dump into the running db
make db-schema                     # dump the live schema for diffing (gitignored)
make db-down ARGS=-v               # stop postgres and drop its data volume
```

Phase 0 lays down the schema, the request-context (`appctx.Caller`) abstraction,
and the app skeleton. Subsequent phases add auth, cube ingestion, decklists, the
analytics engine, and the frontend — see the roadmap.
