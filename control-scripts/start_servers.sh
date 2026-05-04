#!/bin/bash
# start_servers.sh - Start web servers
# Usage: ./start_servers.sh [server_number|all|lb]
# Examples: ./start_servers.sh 1, ./start_servers.sh all

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

NETWORK_NAME="p1-3server_lb_network"

# Docker network mavjudligini tekshirish
ensure_network() {
    if ! docker network inspect "$NETWORK_NAME" >/dev/null 2>&1; then
        echo "Creating Docker network: $NETWORK_NAME"
        docker network create --driver bridge --subnet 172.20.0.0/16 "$NETWORK_NAME" 2>/dev/null || true
    fi
}

start_server() {
    local server_num=$1
    local container_name="web_server_$server_num"
    local host_port=$((8000 + server_num))
    local container_ip="172.20.0.$((10 + server_num))"

    echo "Starting $container_name (port $host_port)..."

    ensure_network

    # Stop and remove if exists
    docker stop "$container_name" 2>/dev/null || true
    docker rm "$container_name" 2>/dev/null || true

    # Start the container with bridge network
    docker run -d \
        --name "$container_name" \
        --network "$NETWORK_NAME" \
        --ip "$container_ip" \
        -p "${host_port}:80" \
        -v "$PROJECT_DIR/index.html:/usr/share/nginx/html/index.html:ro" \
        --restart unless-stopped \
        nginx:alpine

    echo "✓ $container_name started successfully on port $host_port"
}

start_load_balancer() {
    echo "Starting load_balancer..."

    ensure_network

    # Stop and remove if exists
    docker stop load_balancer 2>/dev/null || true
    docker rm load_balancer 2>/dev/null || true

    # Start the load balancer with bridge network
    docker run -d \
        --name load_balancer \
        --network "$NETWORK_NAME" \
        --ip 172.20.0.2 \
        -p 8080:8080 \
        -p 8081:8081 \
        -p 8082:8082 \
        -v "$PROJECT_DIR/nginx.conf:/etc/nginx/nginx.conf:ro" \
        --restart unless-stopped \
        nginx:alpine

    echo "✓ load_balancer started successfully (Admin:8080, Reader:8081, User:8082)"
}

if [ "$1" = "all" ]; then
    echo "Starting all servers..."
    # Discover all web_server entries in docker-compose or start defaults
    for i in 1 2 3; do
        start_server $i
    done
    # Start any additional servers found
    if [ -f "$PROJECT_DIR/docker-compose.yml" ]; then
        for num in $(grep -oP 'web_server_\K[0-9]+' "$PROJECT_DIR/docker-compose.yml" | sort -un); do
            if [ "$num" -gt 3 ]; then
                start_server "$num"
            fi
        done
    fi
    sleep 2
    start_load_balancer
elif [ "$1" = "lb" ]; then
    start_load_balancer
elif [[ "$1" =~ ^[0-9]+$ ]]; then
    start_server "$1"
else
    echo "Usage: $0 [server_number|all|lb]"
    echo "  server_number - Start specific server (1, 2, 3, 4, ...)"
    echo "  all           - Start all servers and load balancer"
    echo "  lb            - Start only load balancer"
    exit 1
fi

echo "✓ Operation completed successfully!"