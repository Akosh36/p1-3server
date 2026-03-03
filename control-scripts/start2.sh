#!/bin/bash
# Start only web_server_2

docker start web_server_2 2>/dev/null || echo "web_server_2 not found or already running"
echo "✓ web_server_2 started"
