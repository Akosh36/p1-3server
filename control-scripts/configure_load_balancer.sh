#!/bin/bash
# configure_load_balancer.sh - Configure load balancer settings
# Usage: ./configure_load_balancer.sh <action> [options]
# Actions: list, add-server, remove-server, set-method

set -e

PROJECT_DIR="/home/akobir/Documents/Projects/DProjects/p1-3server"
cd "$PROJECT_DIR"

NGINX_CONF="nginx.conf"

if [ ! -f "$NGINX_CONF" ]; then
    echo "Error: nginx.conf not found!"
    exit 1
fi

ACTION=$1

case $ACTION in
    list)
        echo "Current load balancer configuration:"
        echo "===================================="
        grep -A 20 "upstream backend" "$NGINX_CONF"
        ;;

    add-server)
        if [ $# -ne 3 ]; then
            echo "Usage: $0 add-server <ip> <port>"
            exit 1
        fi
        IP=$2
        PORT=$3

        # Check if server already exists
        if grep -q "server $IP:$PORT;" "$NGINX_CONF"; then
            echo "Error: Server $IP:$PORT already exists in load balancer!"
            exit 1
        fi

        # Add server to upstream block
        sed -i "/upstream backend {/a\\
        server $IP:$PORT;" "$NGINX_CONF"

        echo "✓ Added server $IP:$PORT to load balancer"
        ;;

    remove-server)
        if [ $# -ne 3 ]; then
            echo "Usage: $0 remove-server <ip> <port>"
            exit 1
        fi
        IP=$2
        PORT=$3

        # Remove server from upstream block
        sed -i "/server $IP:$PORT;/d" "$NGINX_CONF"

        echo "✓ Removed server $IP:$PORT from load balancer"
        ;;

    set-method)
        if [ $# -ne 2 ]; then
            echo "Usage: $0 set-method <method>"
            echo "Methods: round_robin, least_conn, ip_hash, weight"
            exit 1
        fi
        METHOD=$2

        case $METHOD in
            round_robin)
                sed -i 's/upstream backend {/upstream backend {\n    # Load balancing method: round_robin/' "$NGINX_CONF"
                ;;
            least_conn)
                sed -i 's/upstream backend {/upstream backend {\n    least_conn;/' "$NGINX_CONF"
                ;;
            ip_hash)
                sed -i 's/upstream backend {/upstream backend {\n    ip_hash;/' "$NGINX_CONF"
                ;;
            weight)
                echo "For weighted load balancing, manually edit nginx.conf to add 'weight=X' to server directives"
                ;;
            *)
                echo "Error: Unknown method '$METHOD'"
                echo "Available methods: round_robin, least_conn, ip_hash, weight"
                exit 1
                ;;
        esac

        echo "✓ Set load balancing method to $METHOD"
        ;;

    *)
        echo "Usage: $0 <action> [options]"
        echo "Actions:"
        echo "  list                    - List current configuration"
        echo "  add-server <ip> <port>  - Add server to load balancer"
        echo "  remove-server <ip> <port> - Remove server from load balancer"
        echo "  set-method <method>     - Set load balancing method"
        echo "                            Methods: round_robin, least_conn, ip_hash, weight"
        exit 1
        ;;
esac

# Restart load balancer to apply changes
echo "Restarting load balancer to apply changes..."
./restart_servers.sh lb

echo "✓ Load balancer configuration updated successfully!"