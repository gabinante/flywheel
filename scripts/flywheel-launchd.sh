#!/usr/bin/env bash
# Wrapper for launchd — loads .env via varlock, then execs the server binary.
set -euo pipefail

cd /Users/gabeabinante/warrant
exec ./scripts/varlock run -- ./bin/flywheel-server
