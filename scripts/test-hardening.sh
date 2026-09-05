#!/usr/bin/env bash
# Build and exercise Flywheel without the operator's database, sessions, or gh login.
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
run_dir=$(mktemp -d "${TMPDIR:-/tmp}/flywheel-hardening.XXXXXX")
pg_container="flywheel-hardening-pg-$$"
redis_container="flywheel-hardening-redis-$$"
server_pid=''
pg_created=false
redis_created=false
cleanup() {
  if [[ -n "$server_pid" ]]; then kill -TERM "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true; fi
  if $pg_created; then docker rm -f "$pg_container" >/dev/null; fi
  if $redis_created; then docker rm -f "$redis_container" >/dev/null; fi
  echo "Test logs: $run_dir"
}
trap cleanup EXIT
if curl -s --max-time 1 http://127.0.0.1:8091/healthz >/dev/null; then
  echo 'Port 8091 is already in use; stop that test server first.' >&2
  exit 1
fi
docker run -d --rm --name "$pg_container" -e POSTGRES_USER=flywheel_test -e POSTGRES_PASSWORD=flywheel_test -e POSTGRES_DB=flywheel_test -p 127.0.0.1::5432 postgres:16-alpine > "$run_dir/postgres.id"
pg_created=true
docker run -d --rm --name "$redis_container" -p 127.0.0.1::6379 redis:7-alpine > "$run_dir/redis.id"
redis_created=true
pg_port=$(docker port "$pg_container" 5432 | cut -d: -f2)
redis_port=$(docker port "$redis_container" 6379 | cut -d: -f2)
for attempt in {1..60}; do
  if docker exec "$pg_container" pg_isready -U flywheel_test >/dev/null 2>&1; then break; fi
  sleep 1
done
for migration in "$repo"/db/migrations/*.up.sql; do
  docker exec -i "$pg_container" psql -U flywheel_test -d flywheel_test -v ON_ERROR_STOP=1 < "$migration" >> "$run_dir/migrations.log" 2>&1
done
export DATABASE_URL="postgres://flywheel_test:flywheel_test@127.0.0.1:$pg_port/flywheel_test?sslmode=disable"
export REDIS_URL="redis://127.0.0.1:$redis_port/0"
export FLYWHEEL_TEST_DATABASE_URL="$DATABASE_URL"
export FLYWHEEL_E2E_PG_CONTAINER="$pg_container"
export FLYWHEEL_E2E_BASE_URL=http://127.0.0.1:8091
cd "$repo"
go test -count=1 -timeout 120s ./internal/hardening | tee "$run_dir/database-tests.log"
go build -o "$run_dir/server" ./cmd/server
cd "$repo/web"
npm run build > "$run_dir/web-build.log" 2>&1
mkdir -p "$run_dir/holds" "$run_dir/bin" "$run_dir/work" "$run_dir/claude" "$run_dir/codex"
cat > "$run_dir/bin/gh" <<'GH'
#!/bin/sh
if [ "$1 $2" = 'api user' ]; then echo '{"login":"flywheel-test","id":1}'; exit 0; fi
if [ "$1 $2" = 'search prs' ]; then echo '[]'; exit 0; fi
echo 'GitHub is disabled in the isolated browser test server' >&2
exit 1
GH
chmod 700 "$run_dir/bin/gh"
export PATH="$run_dir/bin:$PATH"
export FLYWHEEL_E2E_HARNESS_HOLD_DIR="$run_dir/holds"
export PORT=8091 BASE_URL=http://127.0.0.1:8091 JWT_SECRET=flywheel-isolated-browser-test-secret
export WEB_DIST="$repo/web/dist" WEB_DEV_PROXY_URL=''
export DISPATCH_ENABLED=false ORCHESTRATOR_ENABLED=false REVIEW_ENABLED=false
export REVIEW_WATCH_REQUESTED=false REVIEW_WATCH_AUTHORED=false REVIEW_PUBLISH=false
export FEEDBACK_AUTO_ADDRESS=false LINEAR_SYNC_ENABLED=false LINEAR_API_KEY=''
export REPORT_PROJECT_UPDATES_ENABLED=false REPORT_WEEKLY_ENABLED=false
export SESSIONS_ENABLED=false SESSIONS_CLAUDE_DIR="$run_dir/claude" SESSIONS_CODEX_DIR="$run_dir/codex"
export DISPATCH_WORKTREE_DIR="$run_dir/work" REVIEW_REPO_ROOT="$run_dir/work"
export DISPATCH_AGENT_API_KEY=isolated-fake-key ORCHESTRATOR_AGENT_API_KEY=isolated-fake-key
# Start outside the repository so config cannot load the operator's .env file.
(cd "$run_dir" && exec ./server) > "$run_dir/server.log" 2>&1 &
server_pid=$!
ready=false
for attempt in {1..60}; do
  if ! kill -0 "$server_pid" 2>/dev/null; then cat "$run_dir/server.log"; exit 1; fi
  if curl -sf --max-time 1 http://127.0.0.1:8091/healthz >/dev/null; then ready=true; break; fi
  sleep 1
done
if ! $ready; then echo 'Test server did not become ready.' >&2; exit 1; fi
npx playwright test | tee "$run_dir/playwright.log"
