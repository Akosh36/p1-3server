# HA Web Server Monitoring Dashboard

A real-time graphical monitoring dashboard for the high-availability web server infrastructure.

## Features

✨ **Real-Time Monitoring**
- Live container status tracking
- Backend server health checks
- Response time monitoring
- System resource usage

📊 **Visual Instruments**
- Response time line charts
- System health doughnut charts
- Status indicators
- Resource usage graphs
- Performance metrics

🌐 **Network Information**
- Available access points (ports)
- Local IP address display
- LAN connectivity info
- Server URLs and endpoints

🔍 **Infrastructure Insights**
- CPU, Memory, Disk usage
- Container statistics
- Load balancer status
- Backend server health status
- Response times per server

## Installation

### 1. Install Python Dependencies

```bash
cd /path/to/view
pip install -r requirements.txt
```

Or with better isolation:
```bash
python3 -m venv venv
source venv/bin/activate  # On Windows: venv\Scripts\activate
pip install -r requirements.txt
```

### 2. Make scripts executable (Linux/Mac)
```bash
chmod +x run.sh
```

## Running the Monitoring Dashboard

### Option 1: Using the script
```bash
cd view
./run.sh
```

### Option 2: Direct Python
```bash
cd view
python3 monitor.py
```

## Accessing the Dashboard

Once running, open your browser and visit:

```
http://localhost:5000
```

Or from another machine on the LAN:
```
http://<your-local-ip>:5000
```

The local IP address will be displayed in the dashboard.

## Dashboard Sections

### 1. Host Information
- Hostname and Local IP
- CPU and Memory usage
- Disk usage percentage
- System metrics

### 2. Access Points
- Load Balancer URL (port 80)
- Backend Server 1 (port 8001)
- Backend Server 2 (port 8002)
- Backend Server 3 (port 8003)
- LAN Access IP

### 3. Load Balancer Status
- Health status (UP/DOWN)
- Response time
- Status code
- URL and connectivity

### 4. Backend Servers Health
- Detailed status for each server
- Individual response times
- Health indicators
- Status badges

### 5. Docker Containers Status
- Container names and IDs
- Running/Stopped status
- CPU and Memory usage
- Image information

### 6. Performance Charts
- **Response Times Chart**: Line chart showing response times over time
- **System Health Chart**: Doughnut chart showing resource usage

## What It Shows

### Real-Time Data
- ✓ Container status (Running/Stopped)
- ✓ Response times for each server
- ✓ CPU and Memory per container
- ✓ System CPU, Memory, Disk usage
- ✓ Backend health status
- ✓ Load balancer connectivity

### Networking Information
- ✓ All available ports and URLs
- ✓ Local machine hostname
- ✓ IP address for LAN access
- ✓ Direct backend server URLs for testing

## Monitoring Workflow

1. **Start the infrastructure**
   ```bash
   ../control-scripts/start.sh
   ```

2. **Open the monitoring dashboard**
   ```bash
   ./run.sh
   ```

3. **Access at http://localhost:5000**

4. **View real-time metrics**:
   - Green indicators = Healthy
   - Red indicators = Down/Unhealthy
   - Charts update automatically every 3 seconds

5. **Test failover scenarios**:
   - Stop a server: `../control-scripts/stop1.sh`
   - Watch the dashboard update
   - Observe status changes in real-time

## Testing Scenarios

### Scenario 1: Stop One Server
```bash
# In another terminal:
../control-scripts/stop1.sh

# In dashboard:
- Server 1 will show red (DOWN)
- Response time will increase
- Load balancer will still be UP
- Traffic will route to servers 2 & 3
```

### Scenario 2: Multiple Failures
```bash
../control-scripts/stop2.sh
../control-scripts/stop3.sh

# Dashboard shows:
- Only 1 server healthy
- Load balancer still responsive
- All traffic routed to remaining server
```

### Scenario 3: Server Recovery
```bash
../control-scripts/start1.sh

# Dashboard shows:
- Server 1 comes back online
- Status changes from RED to GREEN
- Response times normalize
```

## Performance Metrics Explained

| Metric | Meaning | Good Range |
|--------|---------|-----------|
| Response Time | Time to get response | < 100ms |
| CPU Usage | Processor load | < 50% |
| Memory Usage | RAM consumption | < 80% |
| Status Code | HTTP response | 200 |

## Architecture Visualization

```
LAN Users / Local Access
        ↓
  http://localhost:5000
  (Monitoring Dashboard)
        ↓
  Docker Containers
  ├── Load Balancer (8080→80)
  ├── Web Server 1 (8001)
  ├── Web Server 2 (8002)
  └── Web Server 3 (8003)
```

## System Requirements

- Python 3.7+
- Docker (running the web servers)
- Modern web browser
- Linux/Mac/Windows with Docker installed

## Ports

- **5000**: Monitoring Dashboard
- **80**: Load Balancer (main entry point)
- **8001-8003**: Backend servers (direct access)

## Troubleshooting

### "Connection refused" errors
```
Make sure the web server infrastructure is running:
../control-scripts/start.sh
```

### Dashboard shows "DOWN" for everything
```
1. Check Docker containers are running:
   docker ps | grep web_server

2. Verify ports are accessible:
   curl http://localhost

3. Check Flask app is responding:
   curl http://localhost:5000
```

### Can't access from another machine
```
1. Find your local IP:
   hostname -I (Linux)
   ifconfig (Mac)
   ipconfig (Windows)

2. Access from other machine:
   http://<your-ip>:5000
```

## Note

This monitoring dashboard does NOT modify the main project. It:
- Connects to existing Docker containers
- Reads their status and metrics
- Displays information only
- Completely separate from the infrastructure

To stop monitoring:
```bash
Press Ctrl+C in the terminal
```

The web servers and load balancer continue running normally.

## Support

For issues with:
- Infrastructure: See ../README.md
- Monitoring: Check Docker and Flask are working
- Ports: Ensure 5000 is not in use
