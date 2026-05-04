#!/bin/bash
# create_docker_server.sh - Create a new Docker web server
# Usage: ./create_docker_server.sh <server_number> <port>
# Example: ./create_docker_server.sh 4 8004

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

NETWORK_NAME="p1-3server_lb_network"

if [ $# -ne 2 ]; then
    echo "Usage: $0 <server_number> <port>"
    echo "Example: $0 4 8004"
    exit 1
fi

SERVER_NUM=$1
PORT=$2
CONTAINER_NAME="web_server_${SERVER_NUM}"
CONTAINER_IP="172.20.0.$((10 + SERVER_NUM))"

# Check if server already exists
if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo "ERROR: Server ${CONTAINER_NAME} already exists!"
    exit 1
fi

# Check if port is already in use
if lsof -i :${PORT} >/dev/null 2>&1; then
    echo "ERROR: Port ${PORT} is already in use!"
    exit 1
fi

echo "Creating Docker server: ${CONTAINER_NAME} on port ${PORT}"

# Ensure network exists
if ! docker network inspect "$NETWORK_NAME" >/dev/null 2>&1; then
    echo "Creating Docker network: $NETWORK_NAME"
    docker network create --driver bridge --subnet 172.20.0.0/16 "$NETWORK_NAME" 2>/dev/null || true
fi

# Create specific NGINX configuration for this backend server
CONF_FILE="nginx_${PORT}.conf"
cat <<EOF > "$CONF_FILE"
server {
    listen 80;
    server_name _;
    location / {
        root   /usr/share/nginx/html;
        index  index.html index.htm;
        add_header X-Server-Port ${PORT};
    }
}
EOF

# Start the Docker container with bridge network
docker run -d \
    --name ${CONTAINER_NAME} \
    --network "$NETWORK_NAME" \
    --ip "$CONTAINER_IP" \
    -p "${PORT}:80" \
    -v "${PROJECT_DIR}/${CONF_FILE}:/etc/nginx/conf.d/default.conf:ro" \
    -v "${PROJECT_DIR}/index.html:/usr/share/nginx/html/index.html:ro" \
    --restart unless-stopped \
    nginx:alpine

# Update nginx.conf upstream block
if [ -f "nginx.conf" ]; then
    if ! grep -q "server ${CONTAINER_NAME}:80" nginx.conf; then
        sed -i "/upstream backend {/a\\        server ${CONTAINER_NAME}:80 max_fails=3 fail_timeout=10s;" nginx.conf
        echo "✓ Added server to load balancer configuration"
        
        # Reload load balancer if running
        if docker ps --format '{{.Names}}' | grep -q "^load_balancer$"; then
            docker exec load_balancer nginx -s reload 2>/dev/null || true
            echo "✓ Load balancer reloaded"
        fi
    fi
fi

echo "SUCCESS: Server ${CONTAINER_NAME} created and running on port ${PORT}"