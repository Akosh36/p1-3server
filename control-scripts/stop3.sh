#!/bin/bash
# Stop only web_server_3

docker stop web_server_3 2>/dev/null || echo "web_server_3 not running"
echo "✓ web_server_3 stopped"
