#!/usr/bin/env bash
# Set up Warrant with Docker Compose: clone if needed, configure .env, then start the stack.
#
# curl|bash trusts the fetched script and TLS to GitHub — same trust model as cloning the repo.
#   curl -fsSL https://raw.githubusercontent.com/gabinante/flywheel/main/scripts/warrant-docker-setup.sh | bash

set -euo pipefail

# Single-line env values only (prevents .env injection / multiline surprises).
sanitize_env_value() {
  local s="$1"
  s="${s//$'\n'/}"
  s="${s//$'\r'/}"
  printf '%s' "$s"
}

# Refs used in git and GitHub archive URLs (branch/tag names).
validate_ref() {
  local r="$1"
  [[ -z "$r" || "$r" == *$'\n'* || "$r" == *[[:cntrl:]]* ]] && return 1
  [[ "$r" =~ ^[a-zA-Z0-9._/-]+$ ]] || return 1
  return 0
}

# Reject path traversal; allows e.g. foo..bar but not /x/../y
validate_path_safe() {
  local d="$1"
  local what="$2"
  if [[ -z "$d" || "$d" == *$'\n'* ]]; then
    echo "error: invalid $what (empty or newline)" >&2
    exit 1
  fi
  if [[ "$d" =~ (^|/)\.\.(/|$) ]]; then
    echo "error: invalid $what (.. path segments not allowed): $d" >&2
    exit 1
  fi
}

random_hex_secret() {
  local out
  out="$(openssl rand -hex 32 2>/dev/null || true)"
  if [[ -z "$out" ]]; then
    echo "error: openssl is required to generate JWT_SECRET (install OpenSSL or create .env manually)" >&2
    exit 1
  fi
  printf '%s' "$out"
}

usage() {
  cat <<'EOF'
Usage: warrant-docker-setup.sh [--ghcr] [--no-build]
  --ghcr      Use pre-built image (docker-compose.ghcr.yml).
  --no-build  Skip image rebuild when building from source.
  -h, --help  Show this help.

curl | bash:  curl -fsSL .../warrant-docker-setup.sh | bash
  Options:     ... | bash -s -- --ghcr

Advanced: WARRANT_REPO, WARRANT_CLONE_DIR, WARRANT_REF, WARRANT_GIT_URL (see script source).
EOF
}

resolve_script_dir() {
  local source="${BASH_SOURCE[0]}"
  while [[ -h "$source" ]]; do
    local dir
    dir="$(cd -P "$(dirname "$source")" && pwd)"
    source="$(readlink "$source")"
    [[ "$source" != /* ]] && source="$dir/$source"
  done
  cd -P "$(dirname "$source")" && pwd
}

materialize_repo() {
  local clone_dir="${WARRANT_CLONE_DIR:-$HOME/warrant}"
  local ref="${WARRANT_REF:-main}"
  local url="${WARRANT_GIT_URL:-https://github.com/gabinante/flywheel.git}"

  validate_path_safe "$clone_dir" WARRANT_CLONE_DIR
  if ! validate_ref "$ref"; then
    echo "error: invalid WARRANT_REF (use branch/tag characters only, no ..): $ref" >&2
    exit 1
  fi

  if [[ -f "$clone_dir/.env.example" ]]; then
    cd -P "$clone_dir" && pwd
    return
  fi

  if [[ -e "$clone_dir" ]]; then
    echo "error: $clone_dir exists but is not a Warrant tree. Remove it or set WARRANT_REPO." >&2
    exit 1
  fi

  if command -v git >/dev/null 2>&1; then
    echo "Cloning Warrant into $clone_dir ..."
    git clone --depth 1 --branch "$ref" -- "$url" "$clone_dir"
  else
    echo "Downloading Warrant into $clone_dir (no git in PATH) ..."
    local parent tmp extracted
    parent="$(dirname "$clone_dir")"
    mkdir -p "$parent"
    tmp="$(mktemp -d)"
    curl -fsSL "https://github.com/gabinante/flywheel/archive/refs/heads/${ref}.tar.gz" | tar xz -C "$tmp"
    extracted="$(find "$tmp" -maxdepth 1 -type d -name 'warrant-*' | head -n 1)"
    if [[ -z "$extracted" || ! -f "$extracted/.env.example" ]]; then
      rm -rf "$tmp"
      echo "error: could not unpack Warrant (install git, or use WARRANT_REF=main)" >&2
      exit 1
    fi
    mv "$extracted" "$clone_dir"
    rm -rf "$tmp"
  fi

  cd -P "$clone_dir" && pwd
}

resolve_repo_root() {
  if [[ -n "${WARRANT_REPO:-}" ]]; then
    validate_path_safe "$WARRANT_REPO" WARRANT_REPO
    cd -P "$WARRANT_REPO" && pwd
    return
  fi
  if [[ -n "${BASH_SOURCE[0]:-}" ]]; then
    local sd
    sd="$(resolve_script_dir)"
    echo "$(cd "$sd/.." && pwd)"
    return
  fi
  materialize_repo
}

# Replace KEY=... line or append (callers must pass sanitized single-line values).
upsert_env() {
  local key="$1" val="$2" file="$3"
  val="$(sanitize_env_value "$val")"
  local tmp out
  tmp="$(mktemp)"
  chmod 600 "$tmp"
  out=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" == "${key}="* ]]; then
      printf '%s=%s\n' "$key" "$val"
      out=1
    else
      printf '%s\n' "$line"
    fi
  done <"$file" >"$tmp"
  if [[ "$out" -eq 0 ]]; then
    printf '%s=%s\n' "$key" "$val" >>"$tmp"
  fi
  mv "$tmp" "$file"
}

tty_read() {
  local prompt="$1"
  local var
  if [[ -r /dev/tty && -w /dev/tty ]]; then
    read -r -p "$prompt" var < /dev/tty || true
  else
    read -r var || true
  fi
  printf '%s' "$(sanitize_env_value "$var")"
}

tty_read_secret() {
  local prompt="$1"
  local var
  if [[ -r /dev/tty && -w /dev/tty ]]; then
    read -r -s -p "$prompt" var < /dev/tty || true
    echo "" > /dev/tty
  else
    read -r -s var || true
    echo ""
  fi
  printf '%s' "$(sanitize_env_value "$var")"
}

setup_env() {
  local env_file="$1"
  local interactive=0
  if [[ -r /dev/tty && -w /dev/tty ]]; then
    interactive=1
  fi

  if [[ -f "$env_file" ]]; then
    echo "Using existing $env_file"
    return
  fi

  cp .env.example "$env_file"

  if [[ "$interactive" -eq 0 ]]; then
    local jwt
    jwt="$(random_hex_secret)"
    upsert_env JWT_SECRET "$jwt" "$env_file"
    chmod 600 "$env_file"
    echo "Created .env (non-interactive: OAuth blank; JWT_SECRET set). Edit $env_file for GitHub OAuth."
    return
  fi

  echo "" > /dev/tty
  echo "Configure .env (GitHub OAuth: https://github.com/settings/developers — callback http://localhost:8080/auth/github/callback)" > /dev/tty
  echo "Leave Client ID empty to skip OAuth for local-only use." > /dev/tty
  echo "" > /dev/tty

  local cid csec jwt
  cid="$(tty_read "GitHub Client ID: ")"
  if [[ -n "$cid" ]]; then
    csec="$(tty_read_secret "GitHub Client Secret: ")"
    upsert_env GITHUB_CLIENT_ID "$cid" "$env_file"
    upsert_env GITHUB_CLIENT_SECRET "$csec" "$env_file"
  fi

  jwt="$(tty_read "JWT secret (Enter to generate random): ")"
  if [[ -z "$jwt" ]]; then
    jwt="$(random_hex_secret)"
    echo "Generated JWT_SECRET." > /dev/tty
  fi
  upsert_env JWT_SECRET "$jwt" "$env_file"
  chmod 600 "$env_file"
  echo "Wrote $env_file" > /dev/tty
}

USE_GHCR=0
NO_BUILD=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --ghcr) USE_GHCR=1 ;;
    --no-build) NO_BUILD=1 ;;
    -h|--help) usage; exit 0 ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
  shift
done

REPO_ROOT="$(resolve_repo_root)"
cd "$REPO_ROOT"

if [[ ! -f .env.example ]]; then
  echo "error: .env.example not found in $REPO_ROOT" >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "error: docker not found in PATH" >&2
  exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
  echo "error: docker compose (v2 plugin) not available" >&2
  exit 1
fi

setup_env ".env"

compose_args=()
if [[ "$USE_GHCR" -eq 1 ]]; then
  compose_args+=(-f docker-compose.ghcr.yml)
fi

up_flags=(-d)
if [[ "$USE_GHCR" -eq 1 ]] || [[ "$NO_BUILD" -eq 1 ]]; then
  :
else
  up_flags=(--build "${up_flags[@]}")
fi

echo "Starting Docker Compose ..."
docker compose "${compose_args[@]}" up "${up_flags[@]}"

echo
echo "http://localhost:8080  —  curl -s http://localhost:8080/healthz"
