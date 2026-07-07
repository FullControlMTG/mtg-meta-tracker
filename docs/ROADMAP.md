# Roadmap

Phased build. Each phase is independently runnable/verifiable.

### Phase 0 — Foundation (this commit)
- [x] Design doc + analytics schema (`docs/DESIGN.md`)
- [x] Postgres schema (`db/schema.sql`, auto-applied by Postgres on first init)
- [x] Repo scaffold: `docker-compose.yml`, `Makefile`, `.env.example`
- [x] Go module + `main.go` wiring skeleton, `appctx`, `config`, `domain/color`
- [x] Next.js app skeleton + `/api` rewrite

### Phase 1 — Backend core + auth
- [x] `store` repositories (pgx) for users/sessions/invites/cubes/cards/jobs
- [x] Sessions + password auth (argon2id) + `appctx` middleware
- [x] Admin-invites-only onboarding (invite → accept-invite) + first-admin bootstrap
- [x] User CRUD endpoints with role/ownership checks
- [ ] Google OAuth (OIDC) → unified session
- [ ] Decklist repositories (deferred to Phase 3)

### Phase 2 — Cube ingestion (multi-cube)
- [x] Scryfall batched client (`/cards/collection`, rate-limited, cached)
- [x] Moxfield adapter + admin cube CRUD (`/api/admin/cubes`) + sync trigger
- [x] Jobs worker + `sync_cube` handler: card cache + `cube_cards` diff
- [ ] Scheduled periodic re-sync

### Phase 3 — Decklists
- [ ] Decklist CRUD, list parsing → `decklist_cards`
- [ ] Color-identity inference (`/api/decklists/infer-colors`) + strategy interface
- [ ] Record update endpoint → enqueue recompute

### Phase 4 — Analytics engine + jobs
- [ ] Jobs worker (poll + dedup/coalesce)
- [ ] Recompute engine → color/card/pair/meta/metric snapshots (incl. shrinkage + Wilson)
- [ ] Read endpoints for analytics
- [ ] Next.js `/api/revalidate` webhook + on-demand revalidation

### Phase 5 — Frontend
- [ ] Analytics dashboard (charts via `dataviz`)
- [ ] Decklist detail (overlaid card fan) with ISR
- [ ] User pages, deck creation page with live inference
- [ ] Auth pages

### Phase 6 — Polish
- [ ] Scheduled cube sync + card refresh jobs
- [ ] Recommendation endpoint (popularity + lift + synergy)
- [ ] Seed/admin bootstrap, tests, CI
