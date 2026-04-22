#!/usr/bin/env bash
# Warrant installer — single-command install via curl|sh.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/gabinante/flywheel/main/scripts/install.sh | bash
#
# Environment variables:
#   WARRANT_VERSION   — version tag to install (default: latest)
#   WARRANT_INSTALL   — installation directory (default: /usr/local/bin)
#   STORAGE_MODE      — set to "embedded" for zero-config mode (default: embedded)
#
set -euo pipefail

REPO="gabinante/flywheel"
VERSION="${WARRANT_VERSION:-latest}"
INSTALL_DIR="${WARRANT_INSTALL:-/usr/local/bin}"
STORAGE_MODE="${STORAGE_MODE:-embedded}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${CYAN}==> ${NC}$*"; }
ok()    { echo -e "${GREEN}==> ${NC}$*"; }
warn()  { echo -e "${YELLOW}==> ${NC}$*"; }
fail()  { echo -e "${RED}==> ${NC}$*" >&2; exit 1; }

# ──────────────────────────────────────────────────────────────────────────
# Detect OS and architecture
# ──────────────────────────────────────────────────────────────────────────
detect_platform() {
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64|amd64) ARCH="amd64" ;;
        arm64|aarch64) ARCH="arm64" ;;
        *) fail "unsupported architecture: $ARCH" ;;
    esac
    case "$OS" in
        linux|darwin) ;;
        *) fail "unsupported OS: $OS" ;;
    esac
}

# ──────────────────────────────────────────────────────────────────────────
# Check dependencies
# ──────────────────────────────────────────────────────────────────────────
check_deps() {
    for cmd in curl tar; do
        if ! command -v "$cmd" &>/dev/null; then
            fail "required command not found: $cmd"
        fi
    done
    # Go is required for building from source until we have pre-built binaries.
    if ! command -v go &>/dev/null; then
        warn "Go not found — will attempt to download pre-built binary"
        HAS_GO=false
    else
        HAS_GO=true
    fi
}

# ──────────────────────────────────────────────────────────────────────────
# Install from source (requires Go)
# ──────────────────────────────────────────────────────────────────────────
install_from_source() {
    info "installing warrant from source..."
    TMPDIR="$(mktemp -d)"
    trap 'rm -rf "$TMPDIR"' EXIT

    if [ "$VERSION" = "latest" ]; then
        CLONE_REF="main"
    else
        CLONE_REF="$VERSION"
    fi

    info "cloning $REPO@$CLONE_REF..."
    git clone --depth 1 --branch "$CLONE_REF" "https://github.com/$REPO.git" "$TMPDIR/warrant" 2>/dev/null \
        || git clone --depth 1 "https://github.com/$REPO.git" "$TMPDIR/warrant"

    cd "$TMPDIR/warrant"
    info "building warrant server..."
    go build -o warrant ./cmd/server

    info "installing to $INSTALL_DIR/warrant..."
    if [ -w "$INSTALL_DIR" ]; then
        cp warrant "$INSTALL_DIR/warrant"
    else
        sudo cp warrant "$INSTALL_DIR/warrant"
    fi
    chmod +x "$INSTALL_DIR/warrant"
}

# ──────────────────────────────────────────────────────────────────────────
# Post-install: create data directory and config
# ──────────────────────────────────────────────────────────────────────────
post_install() {
    DATA_DIR="${WARRANT_DATA_DIR:-$HOME/.warrant/data}"
    mkdir -p "$DATA_DIR"

    ok "warrant installed to $INSTALL_DIR/warrant"
    echo ""
    echo "  Quick start (zero-config embedded mode):"
    echo ""
    echo "    export STORAGE_MODE=embedded"
    echo "    warrant"
    echo ""
    echo "  This starts warrant with SQLite storage — no Postgres or Redis needed."
    echo "  On first launch, an interactive wizard will guide you through setup."
    echo ""
    echo "  To upgrade to production infrastructure later:"
    echo "    1. Set DATABASE_URL to your Postgres connection string"
    echo "    2. Set REDIS_URL to your Redis connection string"
    echo "    3. Remove STORAGE_MODE=embedded"
    echo "    4. Run: warrant"
    echo ""
}

# ──────────────────────────────────────────────────────────────────────────
# Docker install (alternative)
# ──────────────────────────────────────────────────────────────────────────
docker_install() {
    info "pulling warrant Docker image..."
    docker pull "ghcr.io/$REPO:latest"
    ok "Docker image ready"
    echo ""
    echo "  Run with embedded mode:"
    echo ""
    echo "    docker run -it --rm \\"
    echo "      -p 8080:8080 \\"
    echo "      -e STORAGE_MODE=embedded \\"
    echo "      -v \$HOME/.warrant/data:/data \\"
    echo "      -e WARRANT_DATA_DIR=/data \\"
    echo "      ghcr.io/$REPO:latest"
    echo ""
}

# ──────────────────────────────────────────────────────────────────────────
# Main
# ──────────────────────────────────────────────────────────────────────────
main() {
    echo ""
    echo "  ╔═══════════════════════════════════╗"
    echo "  ║   Warrant Installer               ║"
    echo "  ║   Adderall for coding agents.     ║"
    echo "  ╚═══════════════════════════════════╝"
    echo ""

    detect_platform
    check_deps

    if [ "${USE_DOCKER:-}" = "1" ] || [ "${USE_DOCKER:-}" = "true" ]; then
        docker_install
        return
    fi

    if [ "$HAS_GO" = true ]; then
        install_from_source
    else
        warn "Go is not installed. Options:"
        echo "  1) Install Go first: https://go.dev/dl/"
        echo "  2) Use Docker: curl ... | USE_DOCKER=1 bash"
        echo "  3) Use Homebrew: brew install gabinante/tap/warrant"
        exit 1
    fi

    post_install
}

main "$@"
