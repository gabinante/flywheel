#!/usr/bin/env bash
# Check local prerequisites without installing software or changing running services.
set -euo pipefail
cd "$(dirname "$0")/.."
errors=0
for cmd in go node npm python3 docker migrate; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    printf 'Missing prerequisite: %s (see README.md)\n' "$cmd" >&2
    errors=$((errors + 1))
  fi
done
if command -v node >/dev/null 2>&1 && ! node -e 'const [major,minor]=process.versions.node.split(".").map(Number);process.exit(major>22 || (major===22 && minor>=12) ? 0 : 1)'; then
  echo 'Node 22.12 or newer is required.' >&2
  errors=$((errors + 1))
fi
if command -v docker >/dev/null 2>&1; then
  if ! docker info >/dev/null 2>&1 || ! docker compose version >/dev/null 2>&1; then
    echo 'Start Docker with Compose v2 before continuing.' >&2
    errors=$((errors + 1))
  fi
fi
if (( errors > 0 )); then exit 1; fi
if [[ ! -f .env ]]; then
  # Never overwrite the operator's configuration or print the generated secret.
  python3 - <<'PY'
import os, secrets
from pathlib import Path
text = Path('.env.example').read_text().replace('JWT_SECRET=\n', 'JWT_SECRET=' + secrets.token_hex(32) + '\n')
fd = os.open('.env', os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, 'w') as out:
    out.write(text)
PY
  echo 'Created .env with private permissions and a local signing secret.'
fi
./scripts/varlock validate
printf 'Prerequisites ready. Go: %s; Node: %s\n' "$(go version | awk '{print $3}')" "$(node --version)"
