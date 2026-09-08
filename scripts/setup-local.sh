#!/usr/bin/env bash
# Compatibility entry point: start the supported local stack, not the retired embedded server.
set -euo pipefail
cd "$(dirname "$0")/.."
exec make dev
