.PHONY: setup dev dev-backend dev-admin dev-merchant dev-checkout db-up db-down db-migrate db-push db-seed build clean \
	deploy-dev deploy-prod deploy-dev-api deploy-prod-api setup-fly-dev setup-fly-prod

setup:
	npm install
	cp -n .env.example .env || true
	npx prisma generate

dev: db-up
	npx concurrently \
		"npm run dev:backend" \
		"npm run dev:admin" \
		"npm run dev:merchant" \
		"npm run dev:checkout"

dev-backend:
	npm run dev:backend

dev-admin:
	npm run dev:admin

dev-merchant:
	npm run dev:merchant

dev-checkout:
	npm run dev:checkout

db-up:
	docker compose up -d postgres redis

db-down:
	docker compose down

db-migrate:
	npx prisma migrate dev

db-push:
	npx prisma db push

db-seed:
	npx prisma db seed

build:
	npm run build

clean:
	rm -rf node_modules packages/*/node_modules packages/*/dist

# --- Fly.io Deployment ---

deploy-dev:
	./deploy/deploy.sh dev

deploy-prod:
	./deploy/deploy.sh prod

deploy-dev-api:
	./deploy/deploy.sh dev api

deploy-prod-api:
	./deploy/deploy.sh prod api

setup-fly-dev:
	./deploy/setup-fly.sh dev

setup-fly-prod:
	./deploy/setup-fly.sh prod
