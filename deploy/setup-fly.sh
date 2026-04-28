#!/bin/bash
set -euo pipefail

# Usage: ./deploy/setup-fly.sh <dev|prod>
# Creates Fly apps, Postgres cluster, Redis, and sets secrets.
#
# Prerequisites:
#   - fly CLI installed and authenticated (flyctl auth login)
#   - A Fly.io organization

ENV="${1:-dev}"
ORG="${FLY_ORG:-personal}"

if [[ "$ENV" != "dev" && "$ENV" != "prod" ]]; then
  echo "Usage: $0 <dev|prod>"
  exit 1
fi

if [[ "$ENV" == "dev" ]]; then
  PREFIX="ghp-dev"
  PG_SIZE="shared-cpu-1x"
  PG_VOLUME="1"
  REDIS_SIZE="shared-cpu-1x"
else
  PREFIX="ghp"
  PG_SIZE="shared-cpu-2x"
  PG_VOLUME="10"
  REDIS_SIZE="shared-cpu-1x"
fi

echo "=== Setting up $ENV environment (prefix: $PREFIX) ==="
echo ""

# 1. Create apps
for APP in api admin merchant checkout; do
  APP_NAME="${PREFIX}-${APP}"
  if [[ "$APP" == "api" ]]; then
    APP_NAME="${PREFIX}-api"
  fi
  echo "Creating app: $APP_NAME"
  fly apps create "$APP_NAME" --org "$ORG" 2>/dev/null || echo "  (already exists)"
done

# 2. Create Postgres cluster
PG_NAME="${PREFIX}-db"
echo ""
echo "=== Creating Postgres cluster: $PG_NAME ==="
fly postgres create \
  --name "$PG_NAME" \
  --org "$ORG" \
  --region ord \
  --vm-size "$PG_SIZE" \
  --volume-size "$PG_VOLUME" \
  --initial-cluster-size 1 \
  2>/dev/null || echo "  (already exists)"

# Attach Postgres to API
echo "Attaching Postgres to ${PREFIX}-api..."
fly postgres attach "$PG_NAME" --app "${PREFIX}-api" 2>/dev/null || echo "  (already attached)"

# 3. Create Redis (Upstash via Fly)
echo ""
echo "=== Creating Redis ==="
echo "Create Redis manually via: fly redis create --name ${PREFIX}-redis --org $ORG"
echo "Then attach: fly redis attach ${PREFIX}-redis --app ${PREFIX}-api"

# 4. Set secrets on the API app
echo ""
echo "=== Setting secrets on ${PREFIX}-api ==="
echo "Run the following to set required secrets:"
echo ""
cat <<EOF
fly secrets set \\
  JWT_SECRET="$(openssl rand -hex 32)" \\
  JWT_REFRESH_SECRET="$(openssl rand -hex 32)" \\
  ENCRYPTION_KEY="$(openssl rand -hex 16)" \\
  NMI_WEBHOOK_SECRET="your-nmi-webhook-secret" \\
  SEAMLESSCHEX_WEBHOOK_SECRET="your-seamlesschex-webhook-secret" \\
  GHL_CLIENT_ID="your-ghl-client-id" \\
  GHL_CLIENT_SECRET="your-ghl-client-secret" \\
  GHL_SSO_KEY="your-ghl-sso-key" \\
  GHL_APP_ID="your-ghl-app-id" \\
  --app ${PREFIX}-api
EOF

echo ""
echo "=== Setup complete ==="
echo ""
echo "Next steps:"
echo "  1. Set the secrets above"
echo "  2. Create Redis: fly redis create"
echo "  3. Deploy: ./deploy/deploy.sh $ENV"
echo ""
if [[ "$ENV" == "prod" ]]; then
  echo "Custom domains:"
  echo "  fly certs add api.gohighpayment.com --app ghp-api"
  echo "  fly certs add admin.gohighpayment.com --app ghp-admin"
  echo "  fly certs add merchant.gohighpayment.com --app ghp-merchant"
  echo "  fly certs add checkout.gohighpayment.com --app ghp-checkout"
fi
