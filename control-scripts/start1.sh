#!/bin/bash
# Start only web_server_1

docker start web_server_1 2>/dev/null || echo "web_server_1 not found or already running"
echo "✓ web_server_1 started"
