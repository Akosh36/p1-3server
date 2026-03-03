#!/bin/bash
# Stop and remove all containers

docker stop load_balancer web_server_1 web_server_2 web_server_3 2>/dev/null || true
docker rm load_balancer web_server_1 web_server_2 web_server_3 2>/dev/null || true

echo "✓ All containers stopped and removed"
