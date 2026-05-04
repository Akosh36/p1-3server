#!/bin/bash
# update_load_balancer.sh - Automatically update load balancer with all running web servers
# Usage: ./update_load_balancer.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

NGINX_CONF="nginx.conf"
NETWORK_NAME="p1-3server_lb_network"

echo "🔄 Updating load balancer configuration..."

# Get all running web_server containers
echo "Discovering running web servers..."
SERVERS=""
while IFS= read -r container_name; do
    if [[ "$container_name" =~ ^web_server_([0-9]+)$ ]]; then
        SERVERS="${SERVERS}        server ${container_name}:80 max_fails=3 fail_timeout=10s;\n"
        echo "  Found: ${container_name}"
    fi
done < <(docker ps --filter "name=web_server_" --format "{{.Names}}" | sort)

if [ -z "$SERVERS" ]; then
    echo "❌ No web servers found!"
    exit 1
fi

echo ""
echo "Updating nginx.conf upstream block..."

# Create backup
cp "$NGINX_CONF" "${NGINX_CONF}.bak"

# Build new upstream block content
UPSTREAM_CONTENT="    upstream backend {\n${SERVERS}        keepalive 32;\n    }"

# Replace the entire upstream block
python3 -c "
import re
with open('$NGINX_CONF', 'r') as f:
    content = f.read()

# Replace upstream backend block
pattern = r'upstream backend \{[^}]+\}'
replacement = '''upstream backend {
${SERVERS}        keepalive 32;
    }'''
content = re.sub(pattern, replacement, content, flags=re.DOTALL)

with open('$NGINX_CONF', 'w') as f:
    f.write(content)
" 2>/dev/null || {
    # Fallback to sed
    echo "Using sed fallback..."
    cp "${NGINX_CONF}.bak" "$NGINX_CONF"
    # Remove existing server lines in upstream
    sed -i '/upstream backend {/,/}/{/server /d}' "$NGINX_CONF"
    # Add new servers
    sed -i "/upstream backend {/a\\${SERVERS}" "$NGINX_CONF"
}

rm -f "${NGINX_CONF}.bak"

echo "✓ Load balancer configuration updated"
echo ""

# Reload or restart load balancer
if docker ps --format '{{.Names}}' | grep -q "^load_balancer$"; then
    echo "Reloading load balancer..."
    docker exec load_balancer nginx -s reload 2>/dev/null || {
        echo "Reload failed, restarting..."
        "$SCRIPT_DIR/start_servers.sh" lb
    }
fi

echo ""
echo "✅ Load balancer updated successfully!"
echo "Current backend servers:"
docker ps --filter "name=web_server_" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || echo "No web servers running"