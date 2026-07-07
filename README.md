# mtg-meta-tracker

Meta analysis tool for your local playgroup. Supply your own cube decklist data
and gain an understanding of what cards are picked the most, and which perform
the best in your favorite format's metagame.

## Stack

- **Backend** — Go (chi + pgx), PostgreSQL
- **Frontend** — Next.js (App Router): static/ISR decklist pages, interactive
  analytics dashboards
- **Data** — cube pool from a Moxfield list, card data + images from Scryfall
- **Auth** — email/password + Google OAuth, unified server sessions

## Docs

- [`docs/DESIGN.md`](docs/DESIGN.md) — architecture, data model, and the
  analytics schema (color/card/pair stats, shrinkage + Wilson ranking, synergy).
- [`docs/ROADMAP.md`](docs/ROADMAP.md) — phased build plan.

## Quickstart (dev)

```sh
cp .env.example .env            # fill in secrets
make db-up                      # start postgres (applies db/schema.sql on first init)
export DATABASE_URL=postgres://mtg:mtg@localhost:5432/mtg_meta?sslmode=disable

cd backend && go mod tidy && go run ./cmd/server   # :8080
cd frontend && npm install && npm run dev          # :3000  (proxies /api -> :8080)
```

## Database

The schema lives in [`db/schema.sql`](db/schema.sql) and Postgres applies it
automatically the first time it initializes a data directory — no migration step or
tooling required. Because it only runs on an empty `pgdata`, a change to the schema
means editing `db/schema.sql` and re-initializing (`rm -rf pgdata && make db-up` in
dev). Regenerate the file from a live DB with `make db-schema`.

```sh
make db-dump                       # back up schema + data -> db/dump.sql (gitignored)
make db-restore FILE=db/dump.sql   # import a dump into the running db
```

Phase 0 lays down the schema, the request-context (`appctx.Caller`) abstraction,
and the app skeleton. Subsequent phases add auth, cube ingestion, decklists, the
analytics engine, and the frontend — see the roadmap.
