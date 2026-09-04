.PHONY: run run-mcp migrate migrate-create migrate-down test generate docker-up docker-down build build-flywheel-git build-flywheel-mcp web-build varlock-validate install setup-local dev dev-infra dev-stop preflight

VARLOCK := ./scripts/varlock
# Local server port. Non-default so it never collides with other local dev stacks (joinera uses 8080/5432/6379).
PORT ?= 8090

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
	migrate -path db/migrations -database "$${DATABASE_URL:-postgres://flywheel:flywheel@localhost:5439/flywheel?sslmode=disable}" up

build-flywheel-git:
	go build -o flywheel-git ./cmd/flywheel-git

build-flywheel-mcp:
	go build -o flywheel-mcp ./cmd/mcp

migrate-create:
	@name=$${NAME:?Usage: make migrate-create NAME=description}; \
	ts=$$(date -u +%Y%m%d%H%M%S); \
	touch db/migrations/$${ts}_$${name}.up.sql db/migrations/$${ts}_$${name}.down.sql; \
	echo "Created db/migrations/$${ts}_$${name}.{up,down}.sql"

migrate-down:
	migrate -path db/migrations -database "$${DATABASE_URL:-postgres://flywheel:flywheel@localhost:5439/flywheel?sslmode=disable}" down 1

test:
	go test $$(go list ./... | grep -v 'node_modules')

docker-up:
	docker compose up -d

docker-down:
	docker compose down

# Build the warrant binary.
build:
	go build -o warrant ./cmd/server

# Install warrant binary to /usr/local/bin.
install: build
	cp warrant /usr/local/bin/warrant

# ─── Local Development ─────────────────────────────────────────────────────

# Preflight: verify all prerequisites (docker, migrate, nvm/node, .env).
preflight:
	@bash scripts/dev-preflight.sh

# Start dev: preflight, infra (Postgres+Redis), migrations, then native Go server.
# Re-runnable: kills existing server on :$(PORT) before starting fresh.
dev: preflight dev-infra
	@lsof -ti:$(PORT) | xargs kill 2>/dev/null || true
	@sleep 1
	@$(MAKE) migrate 2>/dev/null || true
	$(VARLOCK) run -- go run ./cmd/server

# Start only Docker infra and wait for healthy.
dev-infra:
	@docker compose up -d postgres redis
	@printf "Waiting for Postgres..."
	@until docker compose exec -T postgres pg_isready -U flywheel -d flywheel >/dev/null 2>&1; do printf "."; sleep 1; done
	@echo " ready."
	@printf "Waiting for Redis..."
	@until docker compose exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; do printf "."; sleep 1; done
	@echo " ready."

# Stop the dev server without stopping infra.
dev-stop:
	@lsof -ti:$(PORT) | xargs kill 2>/dev/null || true
	@echo "Server stopped."

# ─── Claude Code Local Setup ─────────────────────────────────────────────────

# One-command setup: build server, provision agent, install MCP proxy, configure Claude Code.
setup-local:
	./scripts/setup-local.sh
