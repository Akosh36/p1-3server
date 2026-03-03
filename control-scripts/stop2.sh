#!/bin/bash
# Stop only web_server_2

docker stop web_server_2 2>/dev/null || echo "web_server_2 not running"
echo "✓ web_server_2 stopped"
