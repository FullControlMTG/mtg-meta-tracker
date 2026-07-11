# Roadmap

Phased build. Each phase is independently runnable/verifiable.

### Phase 0 — Foundation (this commit)
- [x] Design doc + analytics schema (`docs/DESIGN.md`)
- [x] Postgres schema (`db/schema.sql`, auto-applied by Postgres on first init)
- [x] Repo scaffold: `docker-compose.yml`, `Makefile`, `.env.example`
- [x] Go module + `main.go` wiring skeleton, `appctx`, `config`, `domain/color`
- [x] Next.js app skeleton + `/api` rewrite

### Phase 1 — Backend core + auth
- [x] `store` repositories (pgx) for users/sessions/cubes/cards/jobs
- [x] Sessions + password auth (argon2id) + `appctx` middleware
- [x] Admin-created accounts (no public signup) + first-admin bootstrap
- [x] User CRUD endpoints with role/ownership checks
- [ ] Google OAuth (OIDC) → unified session
- [ ] Decklist repositories (deferred to Phase 3)

### Phase 2 — Cube ingestion (multi-cube)
- [x] Scryfall batched client (`/cards/collection`, rate-limited, cached)
- [x] Moxfield adapter + admin cube CRUD (`/api/admin/cubes`) + sync trigger
- [x] Jobs worker + `sync_cube` handler: card cache + `cube_cards` diff
- [ ] Scheduled periodic re-sync

### Phase 3 — Decklists
- [x] Decklist CRUD, list parsing → `decklist_cards`
- [x] Color-identity inference (`/api/decklists/infer-colors`) + resolver strategy (cube pool + Scryfall fallback)
- [x] Record update endpoint → enqueue recompute

### Phase 4 — Analytics engine + jobs
- [x] Jobs worker (poll + dedup/coalesce)  *(worker existed; `recompute_analytics` handler added)*
- [x] Recompute engine → color/card/pair/meta/metric snapshots  *(the derived shrinkage/Wilson/pair-lift stats were later removed — see DESIGN §4.5)*
- [x] Read endpoints for analytics
- [x] Next.js `/api/revalidate` webhook + on-demand revalidation

### Phase 5 — Frontend
- [x] Analytics dashboard (charts via `dataviz`)
- [x] Decklist detail (overlaid card fan) with ISR
- [x] User pages, deck creation page with live inference
- [x] Auth pages (login, settings/change-password, admin user management)

### Phase 6 — Polish
- [ ] Scheduled cube sync + card refresh jobs
- [ ] Per-game (or per-match) results, so card winrates stop inheriting whole deck records (DESIGN §4.5)
- [ ] Seed/admin bootstrap, tests, CI
