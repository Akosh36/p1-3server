# HA Web Server - Monitoring Dashboard Guide

## Overview

A **graphical real-time monitoring dashboard** for the high-availability web server infrastructure has been created in the `view/` folder.

**KEY FEATURES:**
- ✓ Real-time container status monitoring
- ✓ Backend server health checks with response times
- ✓ System resource monitoring (CPU, Memory, Disk)
- ✓ Available ports and IP addresses display
- ✓ Performance charts and graphs
- ✓ Beautiful graphical dashboard
- ✓ Completely separate from the main infrastructure

## Quick Start

### 1. Start the Web Server Infrastructure

```bash
cd control-scripts
./start.sh
```

Wait for all 4 containers to be running.

### 2. Start the Monitoring Dashboard

```bash
cd view
./run.sh
```

Or manually:
```bash
cd view
source venv/bin/activate
python3 monitor.py
```

### 3. Access the Dashboard

Open your browser and go to:
```
http://localhost:5000
```

**From another machine on LAN:**
```
http://<your-local-ip>:5000
```

The local IP will be displayed in the dashboard.

---

## What the Dashboard Shows

### 📊 Real-Time Monitoring

1. **Host Information**
   - Hostname and Local IP address
   - CPU, Memory, Disk usage
   - System performance metrics

2. **Access Points** 
   - Load Balancer: `http://localhost` (port 80)
   - Web Server 1: `localhost:8001`
   - Web Server 2: `localhost:8002`
   - Web Server 3: `localhost:8003`
   - LAN access IP for external machines

3. **Load Balancer Status**
   - Health status (UP/DOWN)
   - Response time in milliseconds
   - HTTP status code
   - Server connectivity

4. **Backend Servers Health**
   - Individual server status
   - Response time for each server
   - Port information
   - Direct server URLs
   - Health indicators (green/red)

5. **Docker Containers Status**
   - Container running status
   - Container IDs
   - Image information
   - CPU usage percentage
   - Memory usage in MB

6. **Performance Charts**
   - Response Time Chart (line graph)
   - System Health Chart (doughnut chart)
   - Real-time data updates

---

## Testing Failover with Dashboard

### Scenario 1: Stop a Single Server

**In Terminal:**
```bash
cd control-scripts
./stop1.sh
```

**In Dashboard:**
- Web Server 1 turns RED (DOWN)
- Response time may increase slightly
- Other servers remain GREEN
- Load balancer still shows as UP
- Watch the charts change in real-time

### Scenario 2: Multiple Server Failures

**In Terminal:**
```bash
./stop1.sh
./stop2.sh
```

**In Dashboard:**
- Servers 1 & 2 turn RED
- Only Server 3 remains GREEN
- Load balancer continues to respond
- All traffic routes to Server 3
- Response times adjust accordingly

### Scenario 3: Server Recovery

**In Terminal:**
```bash
./start1.sh
```

**In Dashboard:**
- Server 1 changes from RED to GREEN
- Response times normalize
- Services marked as "UP"
- Container comes back online

---

## Dashboard Instruments Explained

### Status Badges
- **🟢 UP**: Server is healthy and responding
- **🔴 DOWN**: Server is offline or not responding
- **🟠 TIMEOUT**: Request took too long (>2 seconds)

### Response Time
- Measured in milliseconds (ms)
- Lower is better
- Typical values: 1-50ms
- High values indicate server load or issues

### Health Indicators
- **Green dot** = Running/Healthy
- **Red dot** = Stopped/Unhealthy
- Pulses indicate real-time updates

### Performance Metrics
- **CPU %**: Processor usage
- **Memory MB**: RAM consumption
- **Response Time**: Server latency
- **Status Code**: HTTP response (200 = OK)

---

## Network Information Displayed

### Local Access
```
http://localhost:80      ← Load Balancer (port 80)
http://localhost:8001    ← Web Server 1
http://localhost:8002    ← Web Server 2
http://localhost:8003    ← Web Server 3
```

### LAN Access
The dashboard shows your local IP address. From another machine:
```
http://192.168.x.x:5000  ← Monitoring (port 5000)
http://192.168.x.x:80    ← Load Balancer
```

---

## Chart Interpretations

### Response Time Chart
- **Bottom-left to top-right climb** = Servers slowing down
- **Spikes** = Brief latency increase
- **Drops when server stops** = Connection failures
- **Recovery spikes** = Failover happening

### System Health Chart
- **Larger slices** = Higher resource usage
- **Growing red** = Memory usage increasing
- **Growing blue** = CPU usage increasing
- **Gray section** = Available resources

---

## Monitoring Workflow

1. **Infrastructure Running**
   ```bash
   ../control-scripts/start.sh
   ```

2. **Dashboard Monitoring**
   ```bash
   ./run.sh
   Visit: http://localhost:5000
   ```

3. **Observe Metrics**
   - All servers showing GREEN
   - Response times stable
   - Charts showing normal operation

4. **Test Failover**
   - Stop a server
   - Watch dashboard update
   - Observer status change, response times, charts

5. **Verify Recovery**
   - Restart server
   - Watch it come back online
   - Charts return to normal

---

## Ports Summary

| Port | Service | Access |
|------|---------|--------|
| 80 | Load Balancer | Main entry point |
| 8001 | Web Server 1 | Direct access |
| 8002 | Web Server 2 | Direct access |
| 8003 | Web Server 3 | Direct access |
| 5000 | Monitoring | Dashboard |

---

## File Structure

```
view/
├── monitor.py           ← Flask backend
├── requirements.txt     ← Python dependencies
├── run.sh              ← Start script
├── setup.sh            ← Setup script
├── README.md           ← Detailed docs
├── venv/               ← Virtual environment
└── templates/
    └── dashboard.html  ← Frontend interface
```

---

## Installation & Setup

### One-Time Setup

```bash
cd view
chmod +x setup.sh
./setup.sh
```

### Or Manual Setup

```bash
cd view
python3 -m venv venv
source venv/bin/activate
pip install -r requirements.txt
```

### Running

```bash
cd view
./run.sh
```

The dashboard starts on `http://localhost:5000`

---

## Troubleshooting

### "Connection refused" on Dashboard
```
✓ Make sure web servers are running:
  ../control-scripts/start.sh

✓ Check Docker containers:
  docker ps | grep web_server
```

### All servers showing "DOWN"
```
✓ Wait 5-10 seconds for health checks
✓ Refresh browser (F5)
✓ Check if containers are actually running:
  docker ps
```

### Can't access from another machine
```
✓ Find your local IP:
  hostname -I

✓ Access dashboard:
  http://<your-ip>:5000

✓ Access load balancer:
  http://<your-ip>:80
```

### Port 5000 already in use
```
✓ Stop other Flask apps
✓ Use a different port (edit monitor.py, line ~300)
```

---

## Key Points

✓ **Non-Intrusive**: Monitoring doesn't modify the infrastructure  
✓ **Real-Time**: Updates every 3 seconds  
✓ **Graphical**: Beautiful, responsive UI  
✓ **Comprehensive**: Shows all important metrics  
✓ **Separate**: Independent from main project  

---

## Summary

The monitoring dashboard provides:
- 📊 Real-time status of all containers
- 🌐 All available network access points
- 📈 Performance charts and graphs
- 🎯 Health indicators for each server
- 💾 System resource monitoring
- 🔍 Response time tracking for every server

Perfect for:
- Testing failover scenarios
- Understanding traffic distribution
- Monitoring infrastructure health
- Demonstrating HA capabilities
- Real-time troubleshooting

**Start it with:** `./view/run.sh`
**Access at:** `http://localhost:5000`

