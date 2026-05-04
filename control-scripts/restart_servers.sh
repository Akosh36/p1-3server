#!/bin/bash
# restart_servers.sh - Restart web servers
# Usage: ./restart_servers.sh [server_number|all|lb]
# Examples: ./restart_servers.sh 1, ./restart_servers.sh all

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_DIR"

if [ "$1" = "all" ]; then
    echo "Restarting all servers..."
    "$SCRIPT_DIR/stop_servers.sh" all
    sleep 2
    "$SCRIPT_DIR/start_servers.sh" all
elif [ "$1" = "lb" ]; then
    echo "Restarting load balancer..."
    "$SCRIPT_DIR/stop_servers.sh" lb
    sleep 1
    "$SCRIPT_DIR/start_servers.sh" lb
elif [[ "$1" =~ ^[0-9]+$ ]]; then
    echo "Restarting server $1..."
    "$SCRIPT_DIR/stop_servers.sh" "$1"
    sleep 1
    "$SCRIPT_DIR/start_servers.sh" "$1"
else
    echo "Usage: $0 [server_number|all|lb]"
    echo "  server_number - Restart specific server (any positive integer)"
    echo "  all           - Restart all servers and load balancer"
    echo "  lb            - Restart only load balancer"
    exit 1
fi

echo "✓ Restart completed successfully!"