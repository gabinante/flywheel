FROM node:20-bookworm

# Install networking tools for firewall support and git for repo operations
RUN apt-get update && \
    apt-get install -y --no-install-recommends iptables iproute2 git && \
    rm -rf /var/lib/apt/lists/*

# Install Claude Code globally
RUN npm install -g @anthropic-ai/claude-code

# Create non-root user
RUN useradd -m -s /bin/bash claude && \
    mkdir -p /workspace /repo && \
    chown claude:claude /workspace /repo

# Copy entrypoint
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV NODE_OPTIONS="--max-old-space-size=4096"

WORKDIR /workspace

ENTRYPOINT ["/entrypoint.sh"]
