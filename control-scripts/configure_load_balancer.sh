#!/bin/bash
# configure_load_balancer.sh - Configure load balancer settings
# Usage: ./configure_load_balancer.sh <action> [options]
# Actions: list, add-server, remove-server, set-method

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
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
            echo "Usage: $0 add-server <container_name_or_ip> <port>"
            exit 1
        fi
        SERVER=$2
        PORT=$3

        # Docker network ichida container nomi ishlatiladi
        SERVER_ENTRY="server ${SERVER}:${PORT}"

        if grep -q "$SERVER_ENTRY" "$NGINX_CONF"; then
            echo "Error: Server $SERVER_ENTRY already exists in load balancer!"
            exit 1
        fi

        sed -i "/upstream backend {/a\\        ${SERVER_ENTRY} max_fails=3 fail_timeout=10s;" "$NGINX_CONF"
        echo "✓ Added $SERVER_ENTRY to load balancer"
        ;;

    remove-server)
        if [ $# -ne 3 ]; then
            echo "Usage: $0 remove-server <container_name_or_ip> <port>"
            exit 1
        fi
        SERVER=$2
        PORT=$3

        sed -i "/server ${SERVER}:${PORT}/d" "$NGINX_CONF"
        echo "✓ Removed server ${SERVER}:${PORT} from load balancer"
        ;;

    set-method)
        if [ $# -ne 2 ]; then
            echo "Usage: $0 set-method <method>"
            echo "Methods: round_robin, least_conn, ip_hash"
            exit 1
        fi
        METHOD=$2

        # Remove existing method directives
        sed -i '/^\s*least_conn;/d' "$NGINX_CONF"
        sed -i '/^\s*ip_hash;/d' "$NGINX_CONF"

        case $METHOD in
            round_robin)
                echo "✓ Using round_robin (default, no directive needed)"
                ;;
            least_conn)
                sed -i "/upstream backend {/a\\        least_conn;" "$NGINX_CONF"
                ;;
            ip_hash)
                sed -i "/upstream backend {/a\\        ip_hash;" "$NGINX_CONF"
                ;;
            *)
                echo "Error: Unknown method '$METHOD'"
                echo "Available methods: round_robin, least_conn, ip_hash"
                exit 1
                ;;
        esac

        echo "✓ Set load balancing method to $METHOD"
        ;;

    *)
        echo "Usage: $0 <action> [options]"
        echo "Actions:"
        echo "  list                           - List current configuration"
        echo "  add-server <name_or_ip> <port>  - Add server to load balancer"
        echo "  remove-server <name_or_ip> <port> - Remove server from load balancer"
        echo "  set-method <method>             - Set load balancing method"
        echo "                                    Methods: round_robin, least_conn, ip_hash"
        exit 1
        ;;
esac

# Reload load balancer if running
if docker ps --format '{{.Names}}' | grep -q "^load_balancer$"; then
    echo "Reloading load balancer..."
    docker exec load_balancer nginx -s reload 2>/dev/null || {
        echo "Reload failed, restarting load balancer..."
        "$SCRIPT_DIR/restart_servers.sh" lb
    }
fi

echo "✓ Load balancer configuration updated successfully!"