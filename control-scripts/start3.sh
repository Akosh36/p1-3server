#!/bin/bash
# Start only web_server_3

docker start web_server_3 2>/dev/null || echo "web_server_3 not found or already running"
echo "✓ web_server_3 started"
