#!/bin/bash
set -e

# Apply firewall rules if enabled (default-deny with allowlist)
if [ "$WARRANT_FIREWALL" = "true" ]; then
  iptables -P OUTPUT DROP

  # Allow loopback
  iptables -A OUTPUT -o lo -j ACCEPT

  # Allow established connections
  iptables -A OUTPUT -m state --state ESTABLISHED,RELATED -j ACCEPT

  # Allow DNS
  iptables -A OUTPUT -p udp --dport 53 -j ACCEPT
  iptables -A OUTPUT -p tcp --dport 53 -j ACCEPT

  # Allow host.docker.internal (for MCP -> warrant server)
  if getent hosts host.docker.internal > /dev/null 2>&1; then
    HOST_IP=$(getent hosts host.docker.internal | awk '{print $1}')
    iptables -A OUTPUT -d "$HOST_IP" -j ACCEPT
  fi

  # Allow configured hosts (comma-separated)
  IFS=',' read -ra HOSTS <<< "$WARRANT_ALLOWED_HOSTS"
  for host in "${HOSTS[@]}"; do
    host=$(echo "$host" | xargs)
    if [ -n "$host" ]; then
      iptables -A OUTPUT -d "$host" -p tcp --dport 443 -j ACCEPT
      iptables -A OUTPUT -d "$host" -p tcp --dport 80 -j ACCEPT
    fi
  done

  echo "Firewall rules applied (default-deny with allowlist)"
fi

# Ensure .claude.json exists (claude CLI requires it)
CLAUDE_HOME="/home/claude"
if [ ! -f "$CLAUDE_HOME/.claude.json" ]; then
  # Restore from backup if available
  BACKUP=$(find "$CLAUDE_HOME/.claude/backups" -name '.claude.json.backup.*' 2>/dev/null | head -1)
  if [ -n "$BACKUP" ]; then
    cp "$BACKUP" "$CLAUDE_HOME/.claude.json"
  else
    # Create minimal config
    echo '{}' > "$CLAUDE_HOME/.claude.json"
  fi
  chown claude:claude "$CLAUDE_HOME/.claude.json"
fi

# Drop to claude user and exec the command (preserve environment for API keys)
exec su -p -s /bin/bash claude -c "$*"
