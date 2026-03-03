# Quick Start Guide

## Starting the Infrastructure

```bash
# Make scripts executable
chmod +x control-scripts/*.sh

# Start all containers
./control-scripts/start.sh
```

You should see all 4 containers running (1 load balancer + 3 web servers).

## Accessing the Website

Visit in your browser or use curl:
```bash
curl http://localhost
```

## Testing Failover

### Test 1: Stop One Server
```bash
./control-scripts/stop1.sh
# Website still works - refresh your browser or run curl again
curl http://localhost
```

### Test 2: Stop Two Servers
```bash
./control-scripts/stop2.sh
# Still works with just one server!
curl http://localhost
```

### Test 3: Restart Servers
```bash
docker start web_server_1 web_server_2
# All servers back online
docker ps | grep web_server
```

### Test 4: Kill a Server (Crash Simulation)
```bash
docker kill web_server_1
# Seamlessly routes to remaining servers
for i in {1..5}; do curl -s http://localhost | grep "working normally" && echo "OK"; done
```

## Stopping Everything

```bash
./control-scripts/stop.sh
```

## Key Facts

- **Load Balancer:** Listens on port 80
- **Backend Servers:** Listen on ports 8001, 8002, 8003
- **Health Check:** Detects failed servers after 3 failed attempts
- **Failover Time:** ~10-20 seconds until failed servers are marked down
- **Simultaneous Failures:** Handles up to 2 failed backends seamlessly

## Available Scripts

### Start Scripts
| Script | Purpose |
|--------|---------|
| `./control-scripts/start.sh` | Start all containers (load balancer + 3 web servers) |
| `./control-scripts/start1.sh` | Start only web_server_1 |
| `./control-scripts/start2.sh` | Start only web_server_2 |
| `./control-scripts/start3.sh` | Start only web_server_3 |

### Stop Scripts
| Script | Purpose |
|--------|---------|
| `./control-scripts/stop.sh` | Stop all containers |
| `./control-scripts/stop1.sh` | Stop only web_server_1 |
| `./control-scripts/stop2.sh` | Stop only web_server_2 |
| `./control-scripts/stop3.sh` | Stop only web_server_3 |

## File Structure

```
├── control-scripts/              # Script directory
│   ├── start.sh                 # Start all containers
│   ├── start1.sh                # Start only web_server_1
│   ├── start2.sh                # Start only web_server_2
│   ├── start3.sh                # Start only web_server_3
│   ├── stop.sh                  # Stop all containers
│   ├── stop1.sh                 # Stop only web_server_1
│   ├── stop2.sh                 # Stop only web_server_2
│   └── stop3.sh                 # Stop only web_server_3
├── nginx.conf                    # Load balancer config (with upstream settings)
├── nginx_8001.conf              # Backend server 1 config
├── nginx_8002.conf              # Backend server 2 config
├── nginx_8003.conf              # Backend server 3 config
├── index.html                   # Website content
├── docker-compose.yml           # (Alternative) Docker Compose config
└── README.md                     # Full documentation
```

## Environment Info

- **Architecture:** 1 Load Balancer (Nginx) + 3 Backend Servers (Nginx)
- **Networking:** Host network mode (all containers on host network namespace)
- **Network Communication:** `localhost:8001`, `localhost:8002`, `localhost:8003`
- **Load Balancing:** Round-robin with automatic failover
- **Health Monitoring:** Nginx passive health checks

## Health Check Configuration

In `nginx.conf`, the upstream block includes:
```nginx
upstream backend {
    server 127.0.0.1:8001 max_fails=3 fail_timeout=10s;
    server 127.0.0.1:8002 max_fails=3 fail_timeout=10s;
    server 127.0.0.1:8003 max_fails=3 fail_timeout=10s;
    keepalive 32;
}
```

- `max_fails=3`: Server marked down after 3 failures
- `fail_timeout=10s`: Retry after 10 seconds
- `keepalive 32`: Connection pooling for efficiency
