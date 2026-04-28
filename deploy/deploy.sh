#!/bin/bash
set -euo pipefail

# Usage: ./deploy/deploy.sh <env> [service]
# Examples:
#   ./deploy/deploy.sh dev          # deploy all dev services
#   ./deploy/deploy.sh prod api     # deploy only prod API
#   ./deploy/deploy.sh dev merchant # deploy only dev merchant dashboard

ENV="${1:-dev}"
SERVICE="${2:-all}"

if [[ "$ENV" != "dev" && "$ENV" != "prod" ]]; then
  echo "Usage: $0 <dev|prod> [api|admin|merchant|checkout|all]"
  exit 1
fi

DIR="deploy/$ENV"

deploy_app() {
  local config="$1"
  local name=$(grep '^app = ' "$config" | sed 's/app = "//;s/"//')
  echo ""
  echo "=== Deploying $name from $config ==="
  fly deploy --config "$config" --remote-only
}

if [[ "$SERVICE" == "all" ]]; then
  # Deploy API first (runs migrations), then frontends in parallel
  deploy_app "$DIR/fly.api.toml"
  deploy_app "$DIR/fly.admin.toml" &
  deploy_app "$DIR/fly.merchant.toml" &
  deploy_app "$DIR/fly.checkout.toml" &
  wait
  echo ""
  echo "=== All $ENV services deployed ==="
else
  config="$DIR/fly.${SERVICE}.toml"
  if [[ ! -f "$config" ]]; then
    echo "Config not found: $config"
    echo "Available services: api, admin, merchant, checkout"
    exit 1
  fi
  deploy_app "$config"
fi
