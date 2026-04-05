#!/bin/bash
# upload_html.sh - Upload HTML file to servers
# Usage: ./upload_html.sh <html_file> [server_number|all]
# Examples: ./upload_html.sh newpage.html 1, ./upload_html.sh index.html all

set -e

PROJECT_DIR="/home/akobir/Documents/Projects/DProjects/p1-3server"
cd "$PROJECT_DIR"

if [ $# -lt 1 ] || [ $# -gt 2 ]; then
    echo "Usage: $0 <html_file> [server_number|all]"
    echo "Examples:"
    echo "  $0 newpage.html 1     # Upload to server 1"
    echo "  $0 index.html all     # Upload to all servers"
    exit 1
fi

HTML_FILE=$1
TARGET=$2

if [ ! -f "$HTML_FILE" ]; then
    echo "Error: HTML file '$HTML_FILE' not found!"
    exit 1
fi

# Validate HTML file extension
if [[ ! "$HTML_FILE" =~ \.html$ ]]; then
    echo "Error: File must have .html extension!"
    exit 1
fi

echo "Uploading $HTML_FILE..."

if [ "$TARGET" = "all" ] || [ -z "$TARGET" ]; then
    echo "Uploading to all servers..."
    cp "$HTML_FILE" index.html
    echo "✓ Uploaded to all servers (index.html)"
elif [[ "$TARGET" =~ ^[1-9][0-9]*$ ]]; then
    CONTAINER_NAME="web_server_$TARGET"

    # Check if container exists
    if ! docker ps -a --format 'table {{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        echo "Error: Server ${CONTAINER_NAME} does not exist!"
        exit 1
    fi

    echo "Uploading to $CONTAINER_NAME..."

    # Copy file to container
    docker cp "$HTML_FILE" "${CONTAINER_NAME}:/usr/share/nginx/html/$(basename "$HTML_FILE")"

    # If uploading to index.html, also update local copy
    if [[ "$HTML_FILE" == "index.html" ]]; then
        cp "$HTML_FILE" index.html
    fi

    echo "✓ Uploaded $HTML_FILE to $CONTAINER_NAME"
else
    echo "Error: Invalid target '$TARGET'. Use server number or 'all'"
    exit 1
fi

echo "✓ Upload completed successfully!"