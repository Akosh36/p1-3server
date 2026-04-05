#!/bin/bash
# monitor_hosts.sh - Monitor host systems
# Usage: ./monitor_hosts.sh [host_ip] [action]
# Actions: ping, ssh-check, services, all

set -e

PROJECT_DIR="/home/akobir/Documents/Projects/DProjects/p1-3server"
cd "$PROJECT_DIR"

HOST_IP=${1:-"127.0.0.1"}
ACTION=${2:-"all"}

echo "Monitoring host: $HOST_IP"
echo "=================="

case $ACTION in
    ping)
        echo "Pinging $HOST_IP..."
        ping -c 4 "$HOST_IP"
        ;;

    ssh-check)
        echo "Checking SSH connectivity to $HOST_IP..."
        if [ "$HOST_IP" = "127.0.0.1" ] || [ "$HOST_IP" = "localhost" ]; then
            echo "✓ Local SSH check - assuming OK"
        else
            ssh -o ConnectTimeout=5 -o BatchMode=yes "$HOST_IP" "echo 'SSH connection successful'" 2>/dev/null
            if [ $? -eq 0 ]; then
                echo "✓ SSH connection to $HOST_IP successful"
            else
                echo "✗ SSH connection to $HOST_IP failed"
                exit 1
            fi
        fi
        ;;

    services)
        echo "Checking services on $HOST_IP..."

        # Check our web servers
        for port in 8001 8002 8003 80; do
            if timeout 5 bash -c "echo > /dev/tcp/$HOST_IP/$port" 2>/dev/null; then
                echo "✓ Port $port is open"
            else
                echo "✗ Port $port is closed"
            fi
        done

        # Check Docker
        if [ "$HOST_IP" = "127.0.0.1" ] || [ "$HOST_IP" = "localhost" ]; then
            if docker ps >/dev/null 2>&1; then
                echo "✓ Docker is running"
                docker ps --filter "name=web_server\|load_balancer" --format "table {{.Names}}\t{{.Status}}"
            else
                echo "✗ Docker is not running or not accessible"
            fi
        fi
        ;;

    network)
        echo "Network statistics for $HOST_IP:"
        if [ "$HOST_IP" = "127.0.0.1" ] || [ "$HOST_IP" = "localhost" ]; then
            echo "Network interfaces:"
            ip addr show | grep -E "(inet|link)" | head -10
            echo ""
            echo "Routing table:"
            ip route | head -5
            echo ""
            echo "Network connections:"
            netstat -tuln | head -10
        else
            echo "Remote network monitoring requires SSH access"
        fi
        ;;

    ports)
        echo "Open ports on $HOST_IP:"
        if [ "$HOST_IP" = "127.0.0.1" ] || [ "$HOST_IP" = "localhost" ]; then
            echo "Listening ports:"
            netstat -tuln | grep LISTEN | head -10
            echo ""
            echo "Common service ports check:"
            for port in 22 80 443 8001 8002 8003 8080 3306 5432; do
                if timeout 3 bash -c "echo > /dev/tcp/localhost/$port" 2>/dev/null; then
                    echo "✓ Port $port is open"
                fi
            done
        else
            echo "Remote port scanning requires SSH access"
        fi
        ;;

    processes)
        echo "Process list for $HOST_IP:"
        if [ "$HOST_IP" = "127.0.0.1" ] || [ "$HOST_IP" = "localhost" ]; then
            echo "Top processes by CPU:"
            ps aux --sort=-%cpu | head -10
            echo ""
            echo "Top processes by memory:"
            ps aux --sort=-%mem | head -10
        else
            echo "Remote process monitoring requires SSH access"
        fi
        ;;

    system)
        echo "System information for $HOST_IP:"
        if [ "$HOST_IP" = "127.0.0.1" ] || [ "$HOST_IP" = "localhost" ]; then
            echo "CPU Usage: $(top -bn1 | grep "Cpu(s)" | sed "s/.*, *\([0-9.]*\)%* id.*/\1/" | awk '{print 100 - $1"%"}')"
            echo "Memory Usage: $(free | grep Mem | awk '{printf "%.2f%%", $3/$2 * 100.0}')"
            echo "Disk Usage: $(df / | tail -1 | awk '{print $5}')"
            echo "Load Average: $(uptime | awk -F'load average:' '{ print $2 }')"
        else
            echo "Remote system monitoring requires SSH access"
        fi
        ;;

    all)
        echo "Full monitoring report:"
        echo ""
        $0 "$HOST_IP" ping
        echo ""
        $0 "$HOST_IP" ssh-check
        echo ""
        $0 "$HOST_IP" services
        echo ""
        $0 "$HOST_IP" system
        ;;

    *)
        echo "Usage: $0 [host_ip] [action]"
        echo "Actions:"
        echo "  ping      - Ping the host"
        echo "  ssh-check - Check SSH connectivity"
        echo "  services  - Check service availability"
        echo "  network   - Show network statistics"
        echo "  ports     - Show open ports"
        echo "  processes - Show process list"
        echo "  system    - Show system information"
        echo "  all       - Run all checks (default)"
        exit 1
        ;;
esac

echo ""
echo "✓ Monitoring completed"