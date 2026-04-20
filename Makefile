.PHONY: run run-mcp migrate migrate-down test generate docker-up docker-down build-warrant-git build-warrant-mcp web-build varlock-validate

VARLOCK := ./scripts/varlock

generate:
	go generate ./api/...

web-build:
	cd web && npm ci && npm run build

run:
	$(VARLOCK) run -- go run ./cmd/server

run-mcp:
	$(VARLOCK) run -- go run ./cmd/mcp

# Validate .env against .env.schema without starting anything.
varlock-validate:
	$(VARLOCK) validate

# For Docker Compose, migrations run in the server container. Use this for hosted/non-Docker deploys.
migrate:
	migrate -path db/migrations -database "$${DATABASE_URL:-postgres://warrant:warrant@localhost:5433/warrant?sslmode=disable}" up

build-warrant-git:
	go build -o warrant-git ./cmd/warrant-git

build-warrant-mcp:
	go build -o warrant-mcp ./cmd/mcp

migrate-down:
	migrate -path db/migrations -database "$${DATABASE_URL:-postgres://warrant:warrant@localhost:5433/warrant?sslmode=disable}" down 1

test:
	go test $$(go list ./... | grep -v 'node_modules')

docker-up:
	docker compose up -d

docker-down:
	docker compose down
