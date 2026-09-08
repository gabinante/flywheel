.PHONY: run migrate migrate-create migrate-down test generate docker-up docker-down build web-build varlock-validate setup-local dev dev-infra dev-stop preflight secret-scan

VARLOCK := ./scripts/varlock
# Local server port. Non-default so it never collides with other local dev stacks.
PORT ?= 8090

generate:
	go generate ./api/...

web-build:
	cd web && npm ci && npm run build

run:
	$(VARLOCK) run -- go run ./cmd/server

# Validate .env against .env.schema without starting anything.
varlock-validate:
	$(VARLOCK) validate

# Load the same database configuration as the server. Migration failures are fatal.
migrate:
	$(VARLOCK) run -- sh -c 'exec migrate -path db/migrations -database "$$DATABASE_URL" up'

migrate-create:
	@name=$${NAME:?Usage: make migrate-create NAME=description}; \
	ts=$$(date -u +%Y%m%d%H%M%S); \
	touch db/migrations/$${ts}_$${name}.up.sql db/migrations/$${ts}_$${name}.down.sql; \
	echo "Created db/migrations/$${ts}_$${name}.{up,down}.sql"

migrate-down:
	$(VARLOCK) run -- sh -c 'exec migrate -path db/migrations -database "$$DATABASE_URL" down 1'

test:
	go test $$(go list ./... | grep -v 'node_modules')

docker-up:
	docker compose up -d

docker-down:
	docker compose down

# The server hosts both REST and MCP; run from the repository root to find web/dist.
build:
	mkdir -p bin
	go build -o bin/flywheel-server ./cmd/server

secret-scan:
	./scripts/scan-secrets.sh

# ─── Local Development ─────────────────────────────────────────────────────

# Preflight: verify all prerequisites (docker, migrate, nvm/node, .env).
preflight:
	@bash scripts/dev-preflight.sh

# Build the UI as well as the API. Do not kill another running instance implicitly.
dev: preflight
	$(MAKE) dev-infra
	$(MAKE) migrate
	$(MAKE) web-build
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
	@lsof -tiTCP:$(PORT) -sTCP:LISTEN | xargs kill 2>/dev/null || true
	@echo "Server stopped."

# ─── Claude Code Local Setup ─────────────────────────────────────────────────

# Compatibility alias for the supported local development setup.
setup-local:
	./scripts/setup-local.sh
