#!/bin/bash
# add_server.sh - Add a new web server to the system
# Usage: ./add_server.sh <server_number> <port>
# Example: ./add_server.sh 4 8004

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
    echo "Error: Server ${CONTAINER_NAME} already exists!"
    exit 1
fi

# Check if port is already in use
if lsof -i :${PORT} >/dev/null 2>&1; then
    echo "Error: Port ${PORT} is already in use!"
    exit 1
fi

echo "Adding new server: ${CONTAINER_NAME} on port ${PORT}"

# Update docker-compose.yml to include the new server
if [ -f "docker-compose.yml" ]; then
    if grep -q "  ${CONTAINER_NAME}:" docker-compose.yml; then
        echo "⚠️  Service ${CONTAINER_NAME} already exists in docker-compose.yml"
    else
        # Add the new service before load_balancer
        sed -i "/  load_balancer:/i\\
  ${CONTAINER_NAME}:\\
    image: nginx:alpine\\
    container_name: ${CONTAINER_NAME}\\
    ports:\\
      - \"${PORT}:80\"\\
    volumes:\\
      - ./index.html:/usr/share/nginx/html/index.html:ro\\
    networks:\\
      lb_network:\\
        ipv4_address: ${CONTAINER_IP}\\
    restart: unless-stopped\\
" docker-compose.yml

        # Add to load_balancer depends_on
        sed -i "/depends_on:/a\\      - ${CONTAINER_NAME}" docker-compose.yml
        echo "✓ Updated docker-compose.yml"
    fi
fi

# Update load balancer configuration
if [ -f "nginx.conf" ]; then
    if grep -q "server ${CONTAINER_NAME}:80" nginx.conf; then
        echo "⚠️  Server ${CONTAINER_NAME} already exists in load balancer configuration"
    else
        sed -i "/upstream backend {/a\\        server ${CONTAINER_NAME}:80 max_fails=3 fail_timeout=10s;" nginx.conf
        echo "✓ Updated load balancer configuration"
    fi
fi

echo "✓ Server ${CONTAINER_NAME} added successfully!"
echo "  - Port: ${PORT}"
echo "  - Container: ${CONTAINER_NAME}"
echo "  - Network IP: ${CONTAINER_IP}"
echo ""
echo "To start the new server, run:"
echo "  ./control-scripts/start_servers.sh ${SERVER_NUM}"