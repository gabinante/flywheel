#!/usr/bin/env bash
set -euo pipefail

# Flywheel local setup for Claude Code.
# Builds/starts the server, provisions an agent API key, installs the MCP
# proxy, and configures Claude Code -- all idempotent and safe to re-run.

FLYWHEEL_URL="${FLYWHEEL_URL:-http://localhost:8090}"
CONFIG_ENV="$HOME/.warrant/data/config.env"
PROXY_SRC="$(cd "$(dirname "$0")" && pwd)/flywheel-mcp-proxy"
PROXY_DST="$HOME/.local/bin/flywheel-mcp-proxy"
CLAUDE_JSON="$HOME/.claude.json"

# ── Helpers ──────────────────────────────────────────────────────────────────

info()  { printf '\033[1;34m==> %s\033[0m\n' "$*"; }
ok()    { printf '\033[1;32m  ✓ %s\033[0m\n' "$*"; }
warn()  { printf '\033[1;33m  ! %s\033[0m\n' "$*"; }
fail()  { printf '\033[1;31m  ✗ %s\033[0m\n' "$*"; exit 1; }

check_cmd() {
  command -v "$1" >/dev/null 2>&1
}

truncate_key() {
  local k="$1"
  if [ "${#k}" -gt 12 ]; then
    echo "${k:0:8}...${k: -4}"
  else
    echo "$k"
  fi
}

# ── 1. Check dependencies ───────────────────────────────────────────────────

info "Checking dependencies"
missing=()
for cmd in bash curl python3 go; do
  if check_cmd "$cmd"; then
    ok "$cmd found"
  else
    missing+=("$cmd")
  fi
done
if [ ${#missing[@]} -gt 0 ]; then
  fail "Missing required commands: ${missing[*]}"
fi

# ── 2. Server lifecycle ─────────────────────────────────────────────────────

info "Checking Flywheel server at $FLYWHEEL_URL"

server_healthy() {
  curl -sf "$FLYWHEEL_URL/healthz" >/dev/null 2>&1
}

if server_healthy; then
  ok "Server already running"
  SERVER_STARTED_HERE=false
else
  info "Starting embedded server in background"
  REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
  cd "$REPO_ROOT"

  # Use "go run" instead of building a standalone binary. A standalone
  # binary is unsigned, so macOS treats every rebuild as a new app and
  # re-prompts for network/media permissions. "go run" executes through
  # the already-trusted Go toolchain and avoids the prompts.
  STORAGE_MODE=embedded go run ./cmd/server >/tmp/flywheel-setup.log 2>&1 &
  SERVER_PID=$!
  echo "$SERVER_PID" > /tmp/flywheel-setup.pid

  # Wait for healthy (30s timeout)
  elapsed=0
  while ! server_healthy; do
    if [ $elapsed -ge 30 ]; then
      warn "Server logs (last 20 lines):"
      tail -20 /tmp/flywheel-setup.log 2>/dev/null || true
      fail "Server did not become healthy within 30s"
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  ok "Server started (PID $SERVER_PID)"
  SERVER_STARTED_HERE=true
fi

# ── 3. Obtain API key ───────────────────────────────────────────────────────

info "Obtaining API key"
API_KEY=""

# Try reading from existing config.env
if [ -f "$CONFIG_ENV" ]; then
  API_KEY=$(grep '^DISPATCH_API_KEY=' "$CONFIG_ENV" 2>/dev/null | head -1 | cut -d= -f2- || true)
fi

# Verify the key works against /mcp
key_works() {
  local k="$1"
  [ -n "$k" ] && curl -sf -X POST "$FLYWHEEL_URL/mcp" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: $k" \
    -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"setup-test","version":"0.1"}}}' \
    >/dev/null 2>&1
}

if [ -n "$API_KEY" ] && key_works "$API_KEY"; then
  ok "Existing API key valid: $(truncate_key "$API_KEY")"
else
  if [ -n "$API_KEY" ]; then
    warn "Existing API key invalid, registering new agent"
  fi
  # Register a new agent
  RESP=$(curl -sf -X POST "$FLYWHEEL_URL/agents" \
    -H "Content-Type: application/json" \
    -d '{"name":"claude-code-local","type":"claude"}')
  API_KEY=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['api_key'])")

  if [ -z "$API_KEY" ]; then
    fail "Failed to register agent -- server returned: $RESP"
  fi

  # Persist the key to config.env
  mkdir -p "$(dirname "$CONFIG_ENV")"
  if [ -f "$CONFIG_ENV" ]; then
    # Replace or append
    if grep -q '^DISPATCH_API_KEY=' "$CONFIG_ENV" 2>/dev/null; then
      python3 -c "
import re, sys
p = '$CONFIG_ENV'
txt = open(p).read()
txt = re.sub(r'^DISPATCH_API_KEY=.*$', 'DISPATCH_API_KEY=$API_KEY', txt, flags=re.MULTILINE)
open(p, 'w').write(txt)
"
    else
      echo "DISPATCH_API_KEY=$API_KEY" >> "$CONFIG_ENV"
    fi
  else
    cat > "$CONFIG_ENV" <<ENVEOF
# Flywheel embedded mode configuration
STORAGE_MODE=embedded
DISPATCH_API_KEY=$API_KEY
ENVEOF
  fi
  ok "New agent registered, key: $(truncate_key "$API_KEY")"
fi

# ── 4. Install proxy ────────────────────────────────────────────────────────

info "Installing MCP proxy"
if [ ! -f "$PROXY_SRC" ]; then
  fail "Proxy script not found at $PROXY_SRC"
fi
mkdir -p "$(dirname "$PROXY_DST")"
cp "$PROXY_SRC" "$PROXY_DST"
chmod +x "$PROXY_DST"
ok "Installed to $PROXY_DST"

# ── 5. Configure Claude Code ────────────────────────────────────────────────

info "Configuring Claude Code MCP"

# Clean up old "warrant" entry first, then add/replace "flywheel"
configure_via_cli() {
  # Remove stale warrant entry
  claude mcp remove warrant -s user 2>/dev/null || true

  # Remove existing flywheel entry (so add is idempotent)
  claude mcp remove flywheel -s user 2>/dev/null || true

  # Add flywheel with env vars
  claude mcp add flywheel -s user \
    -e FLYWHEEL_API_KEY="$API_KEY" \
    -e FLYWHEEL_MCP_URL="$FLYWHEEL_URL/mcp" \
    -- "$PROXY_DST"
}

configure_via_json() {
  python3 <<PYEOF
import json, os, sys

path = os.path.expanduser("$CLAUDE_JSON")
cfg = {}
if os.path.exists(path):
    with open(path) as f:
        cfg = json.load(f)

servers = cfg.setdefault("mcpServers", {})

# Remove stale warrant entry
servers.pop("warrant", None)

# Set flywheel entry
servers["flywheel"] = {
    "command": "$PROXY_DST",
    "args": [],
    "env": {
        "FLYWHEEL_API_KEY": "$API_KEY",
        "FLYWHEEL_MCP_URL": "$FLYWHEEL_URL/mcp"
    }
}

with open(path, "w") as f:
    json.dump(cfg, f, indent=2)
    f.write("\n")
PYEOF
}

if check_cmd claude; then
  configure_via_cli
  ok "Configured via 'claude mcp add' (user scope)"
else
  warn "claude CLI not found, editing $CLAUDE_JSON directly"
  configure_via_json
  ok "Configured via $CLAUDE_JSON"
fi

# ── 6. Verify ───────────────────────────────────────────────────────────────

info "Verifying MCP connection"
if key_works "$API_KEY"; then
  ok "API key verified against $FLYWHEEL_URL/mcp"
else
  warn "Verification failed -- server may need a moment"
fi

# ── 7. Summary ──────────────────────────────────────────────────────────────

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
info "Setup complete"
echo ""
echo "  Server:    $FLYWHEEL_URL"
echo "  API key:   $(truncate_key "$API_KEY")"
echo "  Proxy:     $PROXY_DST"
echo "  Config:    $CONFIG_ENV"
echo ""
echo "  Next steps:"
echo "    1. Open Claude Code in any project directory"
echo "    2. Run /mcp to verify the flywheel server appears"
echo "    3. Use flywheel tools: list_projects, list_tickets, etc."
echo ""
if [ "${SERVER_STARTED_HERE:-false}" = true ]; then
  echo "  Server running in background (PID $(cat /tmp/flywheel-setup.pid 2>/dev/null || echo '?'))"
  echo "  Logs: /tmp/flywheel-setup.log"
  echo "  Stop: kill \$(cat /tmp/flywheel-setup.pid)"
fi
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
