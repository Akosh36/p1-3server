#!/bin/bash
# ssh_login.sh - SSH login to hosts
# Usage: ./ssh_login.sh <host_ip> [username] [port]
# Example: ./ssh_login.sh 192.168.1.100 admin 22

set -e

if [ $# -lt 1 ]; then
    echo "Usage: $0 <host_ip> [username] [port]"
    echo "Examples:"
    echo "  $0 192.168.1.100           # SSH as current user"
    echo "  $0 192.168.1.100 admin     # SSH as admin user"
    echo "  $0 192.168.1.100 admin 2222 # SSH as admin on port 2222"
    exit 1
fi

HOST_IP=$1
USERNAME=${2:-"$USER"}
SSH_PORT=${3:-22}

echo "SSH Login Configuration:"
echo "  Host: $HOST_IP"
echo "  User: $USERNAME"
echo "  Port: $SSH_PORT"
echo ""

# Check if host is reachable
echo "Checking connectivity to $HOST_IP..."
if ping -c 1 -W 2 "$HOST_IP" >/dev/null 2>&1; then
    echo "✓ Host $HOST_IP is reachable"
else
    echo "✗ Host $HOST_IP is not reachable"
    exit 1
fi

# Check SSH port
echo "Checking SSH port $SSH_PORT on $HOST_IP..."
if timeout 5 bash -c "echo > /dev/tcp/$HOST_IP/$SSH_PORT" 2>/dev/null; then
    echo "✓ SSH port $SSH_PORT is open"
else
    echo "✗ SSH port $SSH_PORT is closed or filtered"
    exit 1
fi

echo ""
echo "Connecting to $HOST_IP as $USERNAME..."
echo "Type 'exit' to return to this shell"
echo "=================================="

# Execute SSH connection
ssh -p "$SSH_PORT" "${USERNAME}@${HOST_IP}"

echo ""
echo "✓ SSH session ended"