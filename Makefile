.PHONY: db-up db-down db-dump db-restore db-schema backend frontend dev

db-up:        ## start postgres (schema is applied by the backend on startup)
	docker compose up -d db

db-down:
	docker compose down

db-dump:      ## back up schema + data -> db/dump.sql
	docker compose exec -T db pg_dump -U mtg mtg_meta > db/dump.sql

db-restore:   ## import a dump: make db-restore FILE=db/dump.sql
	docker compose exec -T db psql -U mtg -d mtg_meta < $(FILE)

db-schema:    ## dump live schema -> db/schema.generated.sql for diffing (schema.sql is hand-maintained)
	docker compose exec -T db pg_dump --schema-only --no-owner --no-privileges -U mtg mtg_meta > db/schema.generated.sql

backend:
	cd backend && go run ./cmd/server

frontend:
	cd frontend && npm run dev

dev: db-up
	@echo "run 'make backend' and 'make frontend' in separate shells"
