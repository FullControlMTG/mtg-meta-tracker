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
make db-up                      # start postgres (docker)
export DATABASE_URL=postgres://mtg:mtg@localhost:5432/mtg_meta?sslmode=disable
make migrate-up                 # apply backend/migrations (needs golang-migrate)

cd backend && go mod tidy && go run ./cmd/server   # :8080
cd frontend && npm install && npm run dev          # :3000  (proxies /api -> :8080)
```

Phase 0 lays down the schema, the request-context (`appctx.Caller`) abstraction,
and the app skeleton. Subsequent phases add auth, cube ingestion, decklists, the
analytics engine, and the frontend — see the roadmap.
