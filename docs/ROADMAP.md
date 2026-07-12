# Roadmap

Phased build. Each phase is independently runnable/verifiable.

### Phase 0 — Foundation
- [x] Design doc + analytics schema (`docs/DESIGN.md`)
- [x] Postgres schema (`backend/internal/store/schema.sql`, embedded in the binary
      and applied on every backend startup — so it must stay idempotent)
- [x] Repo scaffold: `docker-compose.yml`, `Makefile`, `.env.example`
- [x] Go module + `main.go` wiring skeleton, `appctx`, `config`, `domain/color`
- [x] Next.js app skeleton + `/api` rewrite

### Phase 1 — Backend core + auth
- [x] `store` repositories (pgx) for users/sessions/cubes/cards/jobs
- [x] Sessions + password auth (argon2id) + `appctx` middleware
- [x] Admin-created accounts (no public signup) + first-admin bootstrap
- [x] User CRUD endpoints with role/ownership checks
- [ ] Google OAuth (OIDC) → unified session *(only the `oauth_accounts` table and
      the `GOOGLE_*` config exist; no route is wired)*

### Phase 2 — Cube ingestion (multi-cube)
- [x] Scryfall batched client (`/cards/collection`, rate-limited, cached), resolving
      exact printings
- [x] Admin cube CRUD (`/api/admin/cubes`) + sync trigger
- [x] Jobs worker + `sync_cube` handler: card cache + `cube_cards` diff
- [x] Cube pool from a **pasted list** (`cubes.card_list` + `content_hash`).
      Moxfield's API blocks us, so it is no longer fetched — `internal/moxfield`
      only parses a `publicId` for display.
- [x] Surface unresolved names (`cube_sync_progress.unresolved`) so a typo can't
      silently shrink the pool
- [x] Self-hosted card-image cache (`internal/images`, Docker volume in prod)
- [x] Scheduled periodic re-sync (`internal/jobs/scheduler.go`, every
      `SYNC_INTERVAL_MINUTES`, default 360)

### Phase 3 — Decklists
- [x] Decklist CRUD, list parsing → `decklist_cards`
- [x] Color inference (`/api/decklists/infer-colors`) + resolver strategy (cube pool
      + Scryfall fallback)
- [x] Record update endpoint → enqueue recompute
- [x] Infer colors from **casting costs**, and split sub-threshold colors off as
      splashes (`decklists.splash_colors`) — see DESIGN §8
- [x] Constrain archetype to a fixed enum; drop draws/placement from the record

### Phase 4 — Analytics engine + jobs
- [x] Jobs worker (poll + dedup/coalesce)
- [x] Recompute engine → color/card/pair/meta/metric snapshots
- [x] Read endpoints for analytics
- [x] Next.js `/api/revalidate` webhook + on-demand revalidation
- [x] Scope every stat to a cube
- [x] Remove the derived shrinkage/Wilson/pair-lift stats — see DESIGN §4.5.
      **Do not reintroduce them**; the fix is better input, not better math.

### Phase 5 — Frontend
- [x] Analytics dashboard (hand-rolled SVG charts, sortable tables, MTG palette)
- [x] Decklist detail (overlaid card fan, cards linked to their Scryfall printing)
- [x] User pages, deck creation page with live inference, deck ownership management
- [x] Auth pages (login, settings/change-password, admin user + cube management)
- [x] Card pages (`/cards/[slug]`) and cube pool pages (`/cubes/[id]`)

### Phase 6 — Polish
- [x] Go unit tests (`make test` → `go vet ./... && go test ./...`; ~35 tests across
      `analytics`, `auth`, `decklist`, `domain`, `images`, `ingest`, `moxfield`,
      `scryfall`)
- [x] CI (`Jenkinsfile`: lint/type-check → build → deploy → health + smoke check)
- [ ] **Per-game (or per-match) results**, so card winrates stop inheriting whole
      deck records (DESIGN §4.5). This is the highest-value item left: it is what
      would make a defensible card-power signal possible at all.
- [ ] Test coverage for `store/` and `httpapi/` (currently untested — all existing
      tests are pure/unit and need no database)
- [ ] Any frontend tests (there are none)
- [ ] Wire up `frontend`'s `lint` script — it exists but there is no ESLint config
      or dependency, so it is currently a no-op
