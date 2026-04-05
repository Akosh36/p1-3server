#!/bin/bash
# restart_servers.sh - Restart web servers
# Usage: ./restart_servers.sh [server_number|all]
# Examples: ./restart_servers.sh 1, ./restart_servers.sh all

set -e

PROJECT_DIR="/home/akobir/Documents/Projects/DProjects/p1-3server"
cd "$PROJECT_DIR"

if [ "$1" = "all" ]; then
    echo "Restarting all servers..."
    ./control-scripts/stop_servers.sh all
    sleep 2
    ./control-scripts/start_servers.sh all
elif [ "$1" = "lb" ]; then
    echo "Restarting load balancer..."
    ./control-scripts/stop_servers.sh lb
    sleep 1
    ./control-scripts/start_servers.sh lb
elif [[ "$1" =~ ^[0-9]+$ ]]; then
    echo "Restarting server $1..."
    ./control-scripts/stop_servers.sh "$1"
    sleep 1
    ./control-scripts/start_servers.sh "$1"
else
    echo "Usage: $0 [server_number|all|lb]"
    echo "  server_number - Restart specific server (any positive integer)"
    echo "  all           - Restart all servers and load balancer"
    echo "  lb            - Restart only load balancer"
    exit 1
fi

echo "✓ Restart completed successfully!"