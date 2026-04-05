#!/bin/bash
# start_servers.sh - Start web servers
# Usage: ./start_servers.sh [server_number|all]
# Examples: ./start_servers.sh 1, ./start_servers.sh all

set -e

PROJECT_DIR="/home/akobir/Documents/Projects/DProjects/p1-3server"
cd "$PROJECT_DIR"

start_server() {
    local server_num=$1
    local container_name="web_server_$server_num"
    local nginx_conf="nginx_800${server_num}.conf"
    local port=$((8000 + server_num))

    echo "Starting $container_name..."

    # Create nginx configuration if it doesn't exist
    if [ ! -f "$nginx_conf" ]; then
        echo "Creating nginx configuration for $container_name on port $port"
        cat > "$nginx_conf" << EOF
user nginx;
worker_processes auto;
error_log /var/log/nginx/error.log warn;
pid /var/run/nginx.pid;

events {
    worker_connections 1024;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;
    sendfile on;
    tcp_nopush on;
    keepalive_timeout 65;

    server {
        listen $port;
        server_name _;
        root /usr/share/nginx/html;
        index index.html;
        
        location / {
            try_files \$uri \$uri/ =404;
        }

        location /health {
            access_log off;
            return 200 "healthy\n";
            add_header Content-Type text/plain;
        }
    }
}
EOF
    fi

    # Stop and remove if exists
    docker stop "$container_name" 2>/dev/null || true
    docker rm "$container_name" 2>/dev/null || true

    # Start the container
    docker run -d --name "$container_name" --network host \
        -v "$PWD/index.html:/usr/share/nginx/html/index.html:ro" \
        -v "$PWD/$nginx_conf:/etc/nginx/nginx.conf:ro" \
        nginx:alpine

    echo "✓ $container_name started successfully"
}

start_load_balancer() {
    echo "Starting load_balancer..."

    # Stop and remove if exists
    docker stop load_balancer 2>/dev/null || true
    docker rm load_balancer 2>/dev/null || true

    # Start the load balancer
    docker run -d --name load_balancer --network host \
        -v "$PWD/nginx.conf:/etc/nginx/nginx.conf:ro" \
        nginx:alpine

    echo "✓ load_balancer started successfully"
}

if [ "$1" = "all" ]; then
    echo "Starting all servers..."
    start_server 1
    start_server 2
    start_server 3
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