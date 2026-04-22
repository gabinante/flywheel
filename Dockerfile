# Frontend (Vite + React)
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Go server
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./web/dist
RUN CGO_ENABLED=0 go build -o /flywheel ./cmd/server

# Migrate binary (for entrypoint)
FROM migrate/migrate:v4.18.3 AS migrate

# Run stage
FROM alpine:3.19
RUN apk --no-cache add ca-certificates bash
WORKDIR /app
COPY --from=builder /flywheel .
COPY --from=builder /app/web/dist ./web/dist
COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY db/migrations /app/db/migrations
COPY scripts/varlock /usr/local/bin/varlock
COPY .env.schema /app/.env.schema
EXPOSE 8080
CMD ["sh", "-c", "migrate -path /app/db/migrations -database \"$DATABASE_URL\" up && exec varlock run --schema /app/.env.schema -- ./flywheel"]
