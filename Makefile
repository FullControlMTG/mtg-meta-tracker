.PHONY: db-up db-down db-dump db-restore db-schema backend frontend test dev

# Local dev talks to docker-compose.dev.yml (published port, named volume).
# docker-compose.yml is the production deployment and is not usable locally.
COMPOSE_DEV = docker compose -f docker-compose.dev.yml

db-up:        ## start postgres on :5432 (schema is applied by the backend on startup)
	$(COMPOSE_DEV) up -d db

db-down:      ## stop postgres (add ARGS=-v to also drop the data volume)
	$(COMPOSE_DEV) down $(ARGS)

db-dump:      ## back up schema + data -> db/dump.sql
	$(COMPOSE_DEV) exec -T db pg_dump -U mtg mtg_meta > db/dump.sql

db-restore:   ## import a dump: make db-restore FILE=db/dump.sql
	$(COMPOSE_DEV) exec -T db psql -U mtg -d mtg_meta < $(FILE)

db-schema:    ## dump live schema -> db/schema.generated.sql for diffing
	$(COMPOSE_DEV) exec -T db pg_dump --schema-only --no-owner --no-privileges -U mtg mtg_meta > db/schema.generated.sql

backend:      ## run the API on :8080 (defaults connect to the dev db above)
	cd backend && go run ./cmd/server

frontend:     ## run Next.js on :3000, proxying /api -> :8080
	cd frontend && npm run dev

test:         ## vet + unit tests (no database needed)
	cd backend && go vet ./... && go test ./...

dev: db-up
	@echo "run 'make backend' and 'make frontend' in separate shells"
