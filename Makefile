.PHONY: db-up db-down migrate-up migrate-down backend frontend dev

db-up:        ## start postgres
	docker compose up -d db

db-down:
	docker compose down

# requires golang-migrate (https://github.com/golang-migrate/migrate)
migrate-up:
	migrate -path backend/migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path backend/migrations -database "$(DATABASE_URL)" down 1

backend:
	cd backend && go run ./cmd/server

frontend:
	cd frontend && npm run dev

dev: db-up
	@echo "run 'make backend' and 'make frontend' in separate shells"
