#!/bin/bash
set -e

cd /home/akobir/Documents/Projects/DProjects/p1-3server

# Stop and clean up existing containers
docker stop load_balancer web_server_1 web_server_2 web_server_3 2>/dev/null || true
docker rm load_balancer web_server_1 web_server_2 web_server_3 2>/dev/null || true

# Start backend servers on host network
docker run -d --name web_server_1 --network host \
  -v $PWD/index.html:/usr/share/nginx/html/index.html:ro \
  -v $PWD/nginx_8001.conf:/etc/nginx/nginx.conf:ro \
  nginx:alpine

docker run -d --name web_server_2 --network host \
  -v $PWD/index.html:/usr/share/nginx/html/index.html:ro \
  -v $PWD/nginx_8002.conf:/etc/nginx/nginx.conf:ro \
  nginx:alpine

docker run -d --name web_server_3 --network host \
  -v $PWD/index.html:/usr/share/nginx/html/index.html:ro \
  -v $PWD/nginx_8003.conf:/etc/nginx/nginx.conf:ro \
  nginx:alpine

sleep 2

# Start load balancer on host network
docker run -d --name load_balancer --network host \
  -v $PWD/nginx.conf:/etc/nginx/nginx.conf:ro \
  nginx:alpine

sleep 2

echo "✓ All containers started successfully!"
docker ps | grep -E "load_balancer|web_server"
