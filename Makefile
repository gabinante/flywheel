.PHONY: run run-mcp run-embedded migrate migrate-down test generate docker-up docker-down docker-embedded-up docker-embedded-down build build-flywheel-git build-flywheel-mcp web-build varlock-validate install

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
	migrate -path db/migrations -database "$${DATABASE_URL:-postgres://flywheel:flywheel@localhost:5433/flywheel?sslmode=disable}" up

build-flywheel-git:
	go build -o flywheel-git ./cmd/flywheel-git

build-flywheel-mcp:
	go build -o flywheel-mcp ./cmd/mcp

migrate-down:
	migrate -path db/migrations -database "$${DATABASE_URL:-postgres://flywheel:flywheel@localhost:5433/flywheel?sslmode=disable}" down 1

test:
	go test $$(go list ./... | grep -v 'node_modules')

docker-up:
	docker compose up -d

docker-down:
	docker compose down

# ─── Embedded / Zero-Config Mode ─────────────────────────────────────────────

# Run in embedded mode (SQLite, no Postgres/Redis required).
run-embedded:
	STORAGE_MODE=embedded go run ./cmd/server

# Build the warrant binary.
build:
	go build -o warrant ./cmd/server

# Install warrant binary to /usr/local/bin.
install: build
	cp warrant /usr/local/bin/warrant

# Docker: build and run in embedded mode (zero-config).
docker-embedded-up:
	docker compose -f docker-compose.embedded.yml up -d

docker-embedded-down:
	docker compose -f docker-compose.embedded.yml down
