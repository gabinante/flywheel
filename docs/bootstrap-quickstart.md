# Bootstrap & Zero-Config Quickstart

Get from install to working ticket in under 10 minutes. No Postgres, Redis, or external services needed.

## Install

Choose one method:

### Homebrew (macOS/Linux)

```bash
brew tap gabinante/tap
brew install warrant
```

### curl|sh (Linux/macOS with Go)

```bash
curl -fsSL https://raw.githubusercontent.com/gabinante/flywheel/main/scripts/install.sh | bash
```

### Docker (cross-platform)

```bash
docker run -it --rm \
  -p 8080:8080 \
  -v $HOME/.warrant/data:/data \
  ghcr.io/gabinante/flywheel:embedded
```

Or build locally:

```bash
docker build -f Dockerfile.embedded -t warrant:embedded .
docker run -it --rm -p 8080:8080 -v $HOME/.warrant/data:/data warrant:embedded
```

## First Launch

On first launch (or when no database exists), Warrant runs an interactive setup wizard:

```
╔═════���════════════════════════════════════════════════╗
║           Warrant — First-Run Setup                 ║
║    Adderall for coding agents. Let's get started.   ║
╚══════════════════════════════════════════════════════╝

Repository path [/path/to/repo]:
Anthropic API key (for dispatching agents) [skip]:

Autonomy posture — how much freedom do agents get?
  1) sandbox    — agents run in containers, default-deny network (recommended)
  2) supervised — agents run on host, require approval for risky actions
  3) autonomous — agents run on host, auto-approve when tests pass
Choose [1]:
```

The wizard then performs a **bootstrap scan** of your repository, detecting:
- Languages (Go, TypeScript, Python, Rust, Java, etc.)
- Frameworks (React, Next.js, Express, Vite, etc.)
- Docker (Dockerfiles, docker-compose)
- Kubernetes manifests
- Terraform configs
- CI/CD (GitHub Actions, GitLab CI, Jenkins, CircleCI, Travis)
- Existing .env files (offers migration path)

Output: a draft **project map** you review and accept.

## What Gets Created

After the wizard completes:

| Resource | Description |
|----------|-------------|
| Organization | `default` org for single-operator use |
| Project | Named after your repo directory, with detected tech stack |
| Agent | `bootstrap-agent` with an API key for MCP access |
| Config file | `~/.warrant/data/config.env` with settings |
| Project map | `~/.warrant/data/project-map.json` with scan results |

## Connect Your Agent

```bash
# Claude Code
claude --mcp-server warrant=http://localhost:8080/mcp

# Set the API key (printed during setup)
export WARRANT_API_KEY=wf_...
```

## Zero-Config Defaults

In embedded mode, Warrant uses:

| Component | Default | Production |
|-----------|---------|------------|
| Database | SQLite (`~/.warrant/data/warrant.db`) | Postgres |
| Lease store | In-memory (miniredis) | Redis |
| Findings | Filesystem (`~/.warrant/data/`) | Postgres + object storage |
| Auth | API key only | GitHub OAuth + JWT |
| Secrets | No SOPS/age keys needed | SOPS/age encrypted |
| Event bus | In-process | Redis Pub/Sub |

No `.env` required. No secrets management. Just start the binary.

## Upgrade Paths

When you outgrow embedded mode, migrate incrementally:

### Step 1: Add Postgres

```bash
# Start Postgres (Docker or managed service)
docker run -d --name warrant-pg \
  -e POSTGRES_USER=warrant -e POSTGRES_PASSWORD=warrant -e POSTGRES_DB=warrant \
  -p 5433:5432 postgres:16-alpine

# Switch warrant to Postgres
export DATABASE_URL="postgres://warrant:warrant@localhost:5433/warrant?sslmode=disable"
unset STORAGE_MODE
warrant
```

Data migration: Export from SQLite, import to Postgres (tooling TBD — for now, fresh start with Postgres is recommended for early users).

### Step 2: Add Redis

```bash
# Start Redis
docker run -d --name warrant-redis -p 6379:6379 redis:7-alpine

# Configure
export REDIS_URL="redis://localhost:6379/0"
```

Benefits: Persistent lease storage survives server restarts; pub/sub for multi-instance deployments.

### Step 3: Enable GitHub OAuth

```bash
# Create a GitHub OAuth App: https://github.com/settings/developers
# Callback URL: http://localhost:8080/auth/github/callback
export GITHUB_CLIENT_ID="..."
export GITHUB_CLIENT_SECRET="..."
export JWT_SECRET="$(openssl rand -hex 32)"
```

### Step 4: Full Docker Compose

Once you have Postgres and Redis, use the full stack:

```bash
curl -fsSL https://raw.githubusercontent.com/gabinante/flywheel/main/scripts/warrant-docker-setup.sh | bash
```

This sets up Postgres, Redis, and the Warrant server with migrations.

### Step 5: Enable Agent Dispatch

```bash
export DISPATCH_ENABLED=true
export DISPATCH_API_KEY="wf_..."  # from setup wizard
export DISPATCH_PROJECT_ID="..."   # from setup wizard
export ANTHROPIC_API_KEY="sk-..."  # your Anthropic key
```

## Environment Variables Reference

| Variable | Default | Description |
|----------|---------|-------------|
| `STORAGE_MODE` | (empty) | Set to `embedded` for zero-config SQLite mode |
| `WARRANT_DATA_DIR` | `~/.warrant/data` | Directory for SQLite DB and config |
| `PORT` | `8080` | HTTP server port |
| `DATABASE_URL` | `postgres://...` | Postgres connection (non-embedded) |
| `REDIS_URL` | `redis://localhost:6379/0` | Redis connection (non-embedded) |
| `JWT_SECRET` | (auto-generated) | Secret for JWT token signing |
| `ANTHROPIC_API_KEY` | (empty) | For agent dispatch |
| `DISPATCH_ENABLED` | `false` | Enable agent dispatch worker |
| `GITHUB_CLIENT_ID` | (empty) | GitHub OAuth client ID |
| `GITHUB_CLIENT_SECRET` | (empty) | GitHub OAuth client secret |

## Troubleshooting

### "first run detected" on every start

The wizard runs when `~/.warrant/data/warrant.db` doesn't exist. Ensure the data directory is persisted (Docker volume, stable path).

### Port already in use

```bash
export PORT=9090
warrant
```

### Existing .env file detected

The scanner detects `.env` files in your repo. This is informational — Warrant doesn't read your project's `.env`. It creates its own config at `~/.warrant/data/config.env`.

### Upgrading from embedded to Postgres

Currently a fresh start. Your ticket history lives in the SQLite file at `~/.warrant/data/warrant.db` and can be queried directly if needed.
