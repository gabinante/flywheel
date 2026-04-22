# Homebrew formula for Warrant — work queue for coding agents.
#
# To use as a tap:
#   brew tap gabinante/tap
#   brew install warrant
#
# Or install directly:
#   brew install gabinante/tap/warrant
#
class Warrant < Formula
  desc "Work queue and shared context for AI coding agents"
  homepage "https://github.com/gabinante/flywheel"
  url "https://github.com/gabinante/flywheel.git", branch: "main"
  version "0.1.0"
  license "MIT"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "./cmd/server"
    bin.install "warrant"

    # Also build the MCP binary for IDE integration.
    system "go", "build", "-o", "warrant-mcp", *std_go_args(ldflags: "-s -w"), "./cmd/mcp"
    bin.install "warrant-mcp"
  end

  def post_install
    # Create default data directory.
    (var/"warrant/data").mkpath
  end

  def caveats
    <<~EOS
      Warrant has been installed!

      Quick start (zero-config embedded mode — no Postgres/Redis needed):

        export STORAGE_MODE=embedded
        warrant

      On first launch, a setup wizard will guide you through:
        • Repository path
        • API key configuration
        • Autonomy posture selection

      To upgrade to production infrastructure:
        1. Set DATABASE_URL and REDIS_URL
        2. Remove STORAGE_MODE=embedded
        3. Restart: warrant

      Data directory: #{var}/warrant/data
      Config file:   ~/.warrant/data/config.env
    EOS
  end

  test do
    assert_match "server listening", shell_output("timeout 2 #{bin}/warrant 2>&1 || true")
  end
end
