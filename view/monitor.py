import os
import json
import socket
import psutil
import docker
from flask import Flask, render_template, jsonify
from datetime import datetime
import requests
import threading
import time

app = Flask(__name__)

# Docker client
client = docker.from_env()

# Store monitoring data
monitoring_data = {
    'containers': [],
    'host_info': {},
    'servers_status': [],
    'request_counts': {},
    'last_update': None
}

# Lock for thread-safe access
data_lock = threading.Lock()

def get_local_ip():
    """Get local machine IP address"""
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.connect(("8.8.8.8", 80))
        ip = s.getsockname()[0]
        s.close()
        return ip
    except:
        return "127.0.0.1"

def get_container_info():
    """Get detailed container information"""
    containers_info = []
    try:
        containers = client.containers.list(all=True)
        for container in containers:
            if any(name in container.name for name in ['load_balancer', 'web_server']):
                stats = None
                try:
                    stats = container.stats(stream=False)
                except:
                    stats = None
                
                # Determine if container is running
                is_running = 'running' in container.status.lower()
                
                container_data = {
                    'name': container.name,
                    'id': container.id[:12],
                    'status': container.status,
                    'state': is_running,
                    'image': container.image.tags[0] if container.image.tags else 'unknown',
                    'created': container.attrs['Created'][:10],
                    'ports': container.attrs['NetworkSettings']['Ports'],
                    'cpu_percent': 'N/A',
                    'memory_mb': 'N/A'
                }
                
                if stats:
                    cpu_delta = stats['cpu_stats'].get('cpu_usage', {}).get('total_usage', 0) - \
                               stats['precpu_stats'].get('cpu_usage', {}).get('total_usage', 0)
                    system_delta = stats['cpu_stats'].get('system_cpu_usage', 0) - \
                                  stats['precpu_stats'].get('system_cpu_usage', 0)
                    cpu_percent = (cpu_delta / system_delta * 100.0) if system_delta > 0 else 0
                    
                    memory = stats['memory_stats'].get('usage', 0) / (1024 * 1024)
                    
                    container_data['cpu_percent'] = f"{cpu_percent:.2f}%"
                    container_data['memory_mb'] = f"{memory:.2f}"
                
                containers_info.append(container_data)
    except Exception as e:
        print(f"Error getting container info: {e}")
    
    return containers_info

def check_server_health():
    """Check health of backend servers"""
    servers = [
        {'name': 'web_server_1', 'port': 8001, 'url': 'http://127.0.0.1:8001'},
        {'name': 'web_server_2', 'port': 8002, 'url': 'http://127.0.0.1:8002'},
        {'name': 'web_server_3', 'port': 8003, 'url': 'http://127.0.0.1:8003'},
    ]
    
    servers_status = []
    for server in servers:
        try:
            start = time.time()
            response = requests.get(server['url'], timeout=2)
            response_time = (time.time() - start) * 1000
            
            status = {
                'name': server['name'],
                'port': server['port'],
                'url': server['url'],
                'status': 'UP' if response.status_code == 200 else 'ERROR',
                'response_time_ms': f"{response_time:.2f}",
                'status_code': response.status_code,
                'healthy': response.status_code == 200
            }
        except requests.exceptions.Timeout:
            status = {
                'name': server['name'],
                'port': server['port'],
                'url': server['url'],
                'status': 'TIMEOUT',
                'response_time_ms': '>2000',
                'status_code': 0,
                'healthy': False
            }
        except Exception as e:
            status = {
                'name': server['name'],
                'port': server['port'],
                'url': server['url'],
                'status': 'DOWN',
                'response_time_ms': 'N/A',
                'status_code': 0,
                'healthy': False,
                'error': str(e)
            }
        
        servers_status.append(status)
    
    return servers_status

def check_load_balancer():
    """Check load balancer health"""
    try:
        start = time.time()
        response = requests.get('http://127.0.0.1', timeout=2)
        response_time = (time.time() - start) * 1000
        
        return {
            'name': 'load_balancer',
            'port': 80,
            'url': 'http://127.0.0.1',
            'status': 'UP' if response.status_code == 200 else 'ERROR',
            'response_time_ms': f"{response_time:.2f}",
            'status_code': response.status_code,
            'healthy': response.status_code == 200
        }
    except:
        return {
            'name': 'load_balancer',
            'port': 80,
            'url': 'http://127.0.0.1',
            'status': 'DOWN',
            'response_time_ms': 'N/A',
            'status_code': 0,
            'healthy': False
        }

def get_host_info():
    """Get host system information"""
    try:
        cpu_percent = psutil.cpu_percent(interval=1)
        memory = psutil.virtual_memory()
        
        return {
            'hostname': socket.gethostname(),
            'local_ip': get_local_ip(),
            'cpu_percent': f"{cpu_percent:.1f}%",
            'memory_percent': f"{memory.percent:.1f}%",
            'memory_mb': f"{memory.used / (1024**2):.0f}",
            'memory_total_mb': f"{memory.total / (1024**2):.0f}",
            'disk_percent': f"{psutil.disk_usage('/').percent:.1f}%"
        }
    except Exception as e:
        print(f"Error getting host info: {e}")
        return {}

def update_monitoring_data():
    """Update all monitoring data periodically"""
    while True:
        try:
            with data_lock:
                monitoring_data['containers'] = get_container_info()
                monitoring_data['host_info'] = get_host_info()
                monitoring_data['servers_status'] = check_server_health()
                monitoring_data['load_balancer'] = check_load_balancer()
                monitoring_data['last_update'] = datetime.now().strftime('%Y-%m-%d %H:%M:%S')
        except Exception as e:
            print(f"Error updating monitoring data: {e}")
        
        time.sleep(3)

@app.route('/')
def dashboard():
    """Main dashboard page"""
    return render_template('dashboard.html')

@app.route('/api/status')
def api_status():
    """API endpoint for status data"""
    with data_lock:
        return jsonify(monitoring_data)

@app.route('/api/containers')
def api_containers():
    """API endpoint for container data"""
    with data_lock:
        return jsonify({
            'containers': monitoring_data['containers'],
            'timestamp': monitoring_data['last_update']
        })

@app.route('/api/servers')
def api_servers():
    """API endpoint for server health"""
    with data_lock:
        return jsonify({
            'load_balancer': monitoring_data.get('load_balancer', {}),
            'backends': monitoring_data['servers_status'],
            'timestamp': monitoring_data['last_update']
        })

@app.route('/api/host')
def api_host():
    """API endpoint for host information"""
    with data_lock:
        return jsonify({
            'host': monitoring_data['host_info'],
            'timestamp': monitoring_data['last_update']
        })

if __name__ == '__main__':
    # Start background monitoring thread
    monitor_thread = threading.Thread(target=update_monitoring_data, daemon=True)
    monitor_thread.start()
    
    # Give thread time to collect initial data
    time.sleep(2)
    
    print(f"🎯 Monitoring Dashboard starting...")
    print(f"📊 Access at: http://localhost:5000")
    print(f"🔗 Local IP: {get_local_ip()}")
    
    app.run(host='0.0.0.0', port=5000, debug=False)
