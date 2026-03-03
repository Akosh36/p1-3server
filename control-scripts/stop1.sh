#!/bin/bash
# Stop only web_server_1

docker stop web_server_1 2>/dev/null || echo "web_server_1 not running"
echo "✓ web_server_1 stopped"
