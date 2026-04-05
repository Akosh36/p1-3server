#!/bin/bash
# create_docker_server.sh - Create a new Docker web server
# Usage: ./create_docker_server.sh <server_number> <port>
# Example: ./create_docker_server.sh 4 8004

set -e

PROJECT_DIR="/home/akobir/Documents/Projects/DProjects/p1-3server"
cd "$PROJECT_DIR"

if [ $# -ne 2 ]; then
    echo "Usage: $0 <server_number> <port>"
    echo "Example: $0 4 8004"
    exit 1
fi

SERVER_NUM=$1
PORT=$2
CONTAINER_NAME="web_server_${SERVER_NUM}"

# Check if server already exists
if docker ps -a --format 'table {{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo "ERROR: Server ${CONTAINER_NAME} already exists!"
    exit 1
fi

# Check if port is already in use
if lsof -i :${PORT} >/dev/null 2>&1; then
    echo "ERROR: Port ${PORT} is already in use!"
    exit 1
fi

echo "Creating Docker server: ${CONTAINER_NAME} on port ${PORT}"

# Create nginx configuration for the new server
cat > "nginx_${PORT}.conf" << EOF
server {
    listen ${PORT};
    server_name localhost;

    location / {
        root   /usr/share/nginx/html;
        index  index.html index.htm;
        try_files \$uri \$uri/ =404;
    }

    location /health {
        access_log off;
        return 200 "healthy\n";
        add_header Content-Type text/plain;
    }
}
EOF

# Start the Docker container
docker run -d \
    --name ${CONTAINER_NAME} \
    --network host \
    -e PORT=${PORT} \
    -v ${PROJECT_DIR}/index.html:/usr/share/nginx/html/index.html:ro \
    -v ${PROJECT_DIR}/nginx_${PORT}.conf:/etc/nginx/conf.d/default.conf:ro \
    --restart unless-stopped \
    nginx:alpine

# Add server to load balancer configuration
if [ -f "nginx.conf" ]; then
    # Check if server already exists in load balancer
    if ! grep -q "server 127.0.0.1:${PORT};" nginx.conf; then
        # Add upstream server
        sed -i "/upstream backend {/a\\
        server 127.0.0.1:${PORT} max_fails=3 fail_timeout=10s;" nginx.conf
        echo "✓ Added server to load balancer configuration"
    fi
fi

echo "SUCCESS: Server ${CONTAINER_NAME} created and running on port ${PORT}"