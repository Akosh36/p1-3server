#!/bin/bash
# stop_servers.sh - Stop web servers
# Usage: ./stop_servers.sh [server_number|all|lb]
# Examples: ./stop_servers.sh 1, ./stop_servers.sh all

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

stop_server() {
    local server_num=$1
    local container_name="web_server_$server_num"

    echo "Stopping $container_name..."
    docker stop "$container_name" 2>/dev/null || echo "$container_name not running"
    docker rm "$container_name" 2>/dev/null || echo "$container_name not found"
    echo "✓ $container_name stopped"
}

stop_load_balancer() {
    echo "Stopping load_balancer..."
    docker stop load_balancer 2>/dev/null || echo "load_balancer not running"
    docker rm load_balancer 2>/dev/null || echo "load_balancer not found"
    echo "✓ load_balancer stopped"
}

if [ "$1" = "all" ]; then
    echo "Stopping all servers..."
    stop_load_balancer
    # Stop all web_server containers dynamically
    for container in $(docker ps -a --filter "name=web_server_" --format "{{.Names}}" 2>/dev/null); do
        num=${container##*_}
        if [ -n "$num" ]; then
            stop_server "$num"
        fi
    done
elif [ "$1" = "lb" ]; then
    stop_load_balancer
elif [[ "$1" =~ ^[0-9]+$ ]]; then
    stop_server "$1"
else
    echo "Usage: $0 [server_number|all|lb]"
    echo "  server_number - Stop specific server (1, 2, 3, 4, ...)"
    echo "  all           - Stop all servers and load balancer"
    echo "  lb            - Stop only load balancer"
    exit 1
fi

echo "✓ Operation completed successfully!"