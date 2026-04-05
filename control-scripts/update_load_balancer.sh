#!/bin/bash
# update_load_balancer.sh - Automatically update load balancer with all running web servers
# Usage: ./update_load_balancer.sh

set -e

PROJECT_DIR="/home/akobir/Documents/Projects/DProjects/p1-3server"
cd "$PROJECT_DIR"

NGINX_CONF="nginx.conf"

echo "🔄 Updating load balancer configuration..."

# Get all running web_server containers and their ports
echo "Discovering running web servers..."
SERVERS=""
while IFS= read -r container_name; do
    if [[ "$container_name" =~ ^web_server_([0-9]+)$ ]]; then
        server_num="${BASH_REMATCH[1]}"
        port=$((8000 + server_num))
        SERVERS="${SERVERS}        server 127.0.0.1:${port} max_fails=3 fail_timeout=10s;\n"
        echo "  Found: ${container_name} -> port ${port}"
    fi
done < <(docker ps --filter "name=web_server_" --format "{{.Names}}" | sort)

if [ -z "$SERVERS" ]; then
    echo "❌ No web servers found!"
    exit 1
fi

echo ""
echo "Updating nginx.conf upstream block..."

# Create backup
cp "$NGINX_CONF" "${NGINX_CONF}.backup.$(date +%Y%m%d_%H%M%S)"

# Replace the upstream block
# Find the upstream block and replace its contents
sed -i '/^[[:space:]]*upstream backend {$/,/^[[:space:]]*}[[:space:]]*$/{
    /^[[:space:]]*upstream backend {$/{
        p
        d
    }
    /^[[:space:]]*server /{
        d
    }
    /^[[:space:]]*keepalive /{
        i\
'"$SERVERS"'
        p
        d
    }
    /^[[:space:]]*}[[:space:]]*$/{
        p
        d
    }
}' "$NGINX_CONF"

echo "✓ Load balancer configuration updated"
echo ""
echo "Restarting load balancer..."
./control-scripts/start_servers.sh lb

echo ""
echo "✅ Load balancer updated successfully!"
echo "Current backend servers:"
docker ps --filter "name=web_server_" --format "table {{.Names}}\t{{.Ports}}" | grep -v "NAMES"