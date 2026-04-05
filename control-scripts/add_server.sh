#!/bin/bash
# add_server.sh - Add a new web server
# Usage: ./add_server.sh <server_number> <port>
# Example: ./add_server.sh 4 8004

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
CONF_FILE="nginx_${PORT}.conf"
CONTAINER_NAME="web_server_${SERVER_NUM}"

# Check if server already exists
if docker ps -a --format 'table {{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo "Error: Server ${CONTAINER_NAME} already exists!"
    exit 1
fi

# Check if port is already in use
if lsof -i :${PORT} >/dev/null 2>&1; then
    echo "Error: Port ${PORT} is already in use!"
    exit 1
fi

echo "Adding new server: ${CONTAINER_NAME} on port ${PORT}"

# Create nginx configuration for the new server
cat > "$CONF_FILE" << EOF
events {
    worker_connections 1024;
}

http {
    include       /etc/nginx/mime.types;
    default_type  application/octet-stream;

    sendfile        on;
    keepalive_timeout  65;

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
}
EOF

echo "✓ Created nginx configuration: ${CONF_FILE}"

# Update docker-compose.yml to include the new server
if [ -f "docker-compose.yml" ]; then
    # Check if service already exists
    if grep -q "  ${CONTAINER_NAME}:" docker-compose.yml; then
        echo "⚠️  Service ${CONTAINER_NAME} already exists in docker-compose.yml"
    else
        # Add the new service to docker-compose.yml
        sed -i "/  load_balancer:/i\\
  ${CONTAINER_NAME}:\\
    image: nginx:alpine\\
    container_name: ${CONTAINER_NAME}\\
    network_mode: host\\
    environment:\\
      - PORT=${PORT}\\
    volumes:\\
      - ./index.html:/usr/share/nginx/html/index.html:ro\\
      - ./${CONF_FILE}:/etc/nginx/conf.d/default.conf:ro\\
    restart: unless-stopped\\
" docker-compose.yml
        echo "✓ Updated docker-compose.yml"
    fi
fi

# Update load balancer configuration to include the new server
if [ -f "nginx.conf" ]; then
    # Check if server already exists in load balancer
    if grep -q "server 127.0.0.1:${PORT};" nginx.conf; then
        echo "⚠️  Server 127.0.0.1:${PORT} already exists in load balancer configuration"
    else
        # Add upstream server
        sed -i "/upstream backend {/a\\
        server 127.0.0.1:${PORT};" nginx.conf
        echo "✓ Updated load balancer configuration"
    fi
fi

echo "✓ Server ${CONTAINER_NAME} added successfully!"
echo "  - Configuration: ${CONF_FILE}"
echo "  - Port: ${PORT}"
echo "  - Container: ${CONTAINER_NAME}"
echo ""
echo "To start the new server, run:"
echo "  ./start_servers.sh ${SERVER_NUM}"