import os
import json
import socket
import psutil
import docker
import shlex
import subprocess
from flask import Flask, render_template, jsonify, request, flash, redirect, url_for, get_flashed_messages
from datetime import datetime
import requests
import threading
import time
from werkzeug.utils import secure_filename

app = Flask(__name__)
app.secret_key = 'ha-web-server-monitoring-key-2024'

# Configuration
PROJECT_DIR = "/home/akobir/Documents/Projects/DProjects/p1-3server"
CONTROL_SCRIPTS_DIR = os.path.join(PROJECT_DIR, "control-scripts")
UPLOAD_FOLDER = os.path.join(PROJECT_DIR, "uploads")
ALLOWED_EXTENSIONS = {'html'}

app.config['UPLOAD_FOLDER'] = UPLOAD_FOLDER

# Ensure upload directory exists
os.makedirs(UPLOAD_FOLDER, exist_ok=True)

# Docker client (with error handling)
docker_available = False
use_subprocess = True
client = None

try:
    # First check if Docker daemon is accessible via subprocess
    result = subprocess.run(['docker', 'version'], capture_output=True, text=True, timeout=5)
    if result.returncode == 0:
        # Docker daemon is accessible, try Python library
        import docker
        client = docker.from_env()
        # Test the connection
        client.ping()
        docker_available = True
        use_subprocess = False
        print("✅ Docker Python library connected successfully")
    else:
        raise Exception("Docker daemon not accessible")
except Exception as e:
    print(f"⚠️  Docker Python library not available ({e}), using subprocess fallback")
    docker_available = True  # We'll use subprocess
    use_subprocess = True

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
    if not docker_available:
        return []

    if use_subprocess:
        # Use subprocess to get container info
        try:
            result = subprocess.run(['docker', 'ps', '-a', '--format', '{{json .}}'],
                                  capture_output=True, text=True, timeout=10)
            if result.returncode == 0:
                containers_info = []
                lines = result.stdout.strip().split('\n')
                for line in lines:
                    if line.strip():
                        try:
                            container = json.loads(line)
                            # Only include our containers
                            if any(name in container.get('Names', '') for name in ['load_balancer', 'web_server']):
                                containers_info.append({
                                    'name': container.get('Names', '').replace('/', ''),
                                    'id': container.get('ID', '')[:12],
                                    'status': container.get('Status', ''),
                                    'state': 'running' in container.get('Status', ''),
                                    'image': container.get('Image', ''),
                                    'created': '',  # Not available in this format
                                    'ports': '',    # Not available in this format
                                    'cpu_percent': 'N/A',
                                    'memory_mb': 'N/A'
                                })
                        except json.JSONDecodeError:
                            continue
                return containers_info
            else:
                print(f"Docker command failed: {result.stderr}")
                return []
        except Exception as e:
            print(f"Error getting container info via subprocess: {e}")
            return []
    else:
        # Use Python Docker library
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
    servers = []

    # Get all web_server containers (running and stopped)
    if docker_available:
        if use_subprocess:
            try:
                result = subprocess.run(['docker', 'ps', '-a', '--filter', 'name=web_server_', '--format', '{{json .}}'],
                                      capture_output=True, text=True, timeout=5)
                if result.returncode == 0:
                    lines = result.stdout.strip().split('\n')
                    for line in lines:
                        if line.strip():
                            try:
                                container_data = json.loads(line)
                                container_name = container_data.get('Names', '').replace('/', '')
                                container_status = container_data.get('Status', '')
                                if container_name:
                                    try:
                                        server_num = int(container_name.split('_')[-1])
                                        port = 8000 + server_num
                                        servers.append({
                                            'name': container_name,
                                            'port': port,
                                            'url': f'http://127.0.0.1:{port}',
                                            'status_text': container_status
                                        })
                                    except (ValueError, IndexError):
                                        continue
                            except json.JSONDecodeError:
                                continue
            except Exception as e:
                print(f"Error getting containers via subprocess: {e}")
        else:
            # Use Docker Python library
            try:
                containers = client.containers.list(all=True, filters={'name': 'web_server_'})
                for container in containers:
                    container_name = container.name
                    try:
                        server_num = int(container_name.split('_')[-1])
                        port = 8000 + server_num
                        servers.append({
                            'name': container_name,
                            'port': port,
                            'url': f'http://127.0.0.1:{port}',
                            'status_text': container.status
                        })
                    except (ValueError, IndexError):
                        continue
            except Exception as e:
                print(f"Error getting containers via Docker library: {e}")

    # Fallback to hardcoded servers if no containers found
    if not servers:
        servers = [
            {'name': 'web_server_1', 'port': 8001, 'url': 'http://127.0.0.1:8001', 'status_text': 'unknown'},
            {'name': 'web_server_2', 'port': 8002, 'url': 'http://127.0.0.1:8002', 'status_text': 'unknown'},
            {'name': 'web_server_3', 'port': 8003, 'url': 'http://127.0.0.1:8003', 'status_text': 'unknown'},
        ]

    servers_status = []
    for server in servers:
        status_text = server.get('status_text', '')
        if status_text and 'up' not in status_text.lower():
            status = {
                'name': server['name'],
                'port': server['port'],
                'url': server['url'],
                'status': 'STOPPED' if 'exited' in status_text.lower() or 'created' in status_text.lower() else 'DOWN',
                'response_time_ms': 'N/A',
                'status_code': 0,
                'healthy': False,
                'status_text': status_text
            }
        else:
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
                    'healthy': response.status_code == 200,
                    'status_text': status_text
                }
            except requests.exceptions.Timeout:
                status = {
                    'name': server['name'],
                    'port': server['port'],
                    'url': server['url'],
                    'status': 'TIMEOUT',
                    'response_time_ms': '>2000',
                    'status_code': 0,
                    'healthy': False,
                    'status_text': status_text
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
                    'error': str(e),
                    'status_text': status_text
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

def allowed_file(filename):
    """Check if file has allowed extension"""
    return '.' in filename and filename.rsplit('.', 1)[1].lower() in ALLOWED_EXTENSIONS

def run_script(script_name, *args):
    """Run a control script and return output"""
    script_path = os.path.join(CONTROL_SCRIPTS_DIR, script_name)

    if not os.path.exists(script_path):
        return False, f"Script {script_name} not found"

    try:
        cmd = [script_path] + list(args)
        result = subprocess.run(cmd, capture_output=True, text=True, cwd=PROJECT_DIR, timeout=60)

        success = result.returncode == 0
        output = result.stdout if success else result.stderr

        return success, output.strip()
    except subprocess.TimeoutExpired:
        return False, "Script execution timed out"
    except Exception as e:
        return False, f"Error running script: {str(e)}"


def docker_stop_container(container_name):
    if not docker_available:
        return False, "Docker not available"

    try:
        if use_subprocess:
            result = subprocess.run(['docker', 'stop', container_name], capture_output=True, text=True, timeout=20)
            if result.returncode == 0:
                return True, result.stdout.strip()
            stderr = (result.stderr or result.stdout).strip()
            if 'is not running' in stderr.lower() or 'already stopped' in stderr.lower():
                return True, stderr
            return False, stderr
        else:
            container = client.containers.get(container_name)
            container.stop()
            return True, f"Stopped {container_name}"
    except Exception as e:
        message = str(e)
        if 'is not running' in message.lower() or 'not running' in message.lower():
            return True, message
        return False, message


def stop_containers(target):
    target = str(target)
    names = []

    if target == 'all':
        names.append('load_balancer')
        if docker_available:
            try:
                if use_subprocess:
                    result = subprocess.run(['docker', 'ps', '-a', '--filter', 'name=web_server_', '--format', '{{.Names}}'],
                                            capture_output=True, text=True, timeout=10)
                    if result.returncode == 0:
                        names.extend([name.strip() for name in result.stdout.split('\n') if name.strip()])
                else:
                    containers = client.containers.list(all=True, filters={'name': 'web_server_'})
                    names.extend([container.name for container in containers])
            except Exception as e:
                return False, str(e)
    elif target == 'lb':
        names = ['load_balancer']
    elif target.isdigit():
        names = [f'web_server_{target}']
    else:
        return False, 'Invalid target'

    if not names:
        return False, 'No matching containers found'

    messages = []
    for name in names:
        success, output = docker_stop_container(name)
        messages.append(f"{name}: {output}")
        if not success:
            return False, '; '.join(messages)

    return True, '; '.join(messages)


def list_control_scripts():
    """Return a list of available control scripts."""
    try:
        scripts = [f for f in os.listdir(CONTROL_SCRIPTS_DIR)
                   if os.path.isfile(os.path.join(CONTROL_SCRIPTS_DIR, f)) and f.endswith('.sh')]
        return sorted(scripts)
    except Exception as e:
        print(f"Error listing control scripts: {e}")
        return []

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
    with data_lock:
        return render_template('dashboard.html', last_update=monitoring_data.get('last_update'))

@app.route('/view/dashboard')
def dashboard_view():
    with data_lock:
        return render_template('dashboard.html', last_update=monitoring_data.get('last_update'))

@app.route('/view/management')
def management():
    with data_lock:
        containers = monitoring_data.get('containers', [])
        server_targets = [
            {'label': 'Barcha serverlar', 'value': 'all'},
            {'label': 'Load Balancer', 'value': 'lb'}
        ]
        web_containers = []
        for container in containers:
            name = container.get('name', '')
            if name.startswith('web_server_'):
                try:
                    server_num = int(name.rsplit('_', 1)[-1])
                    web_containers.append((server_num, name))
                except ValueError:
                    continue
        for server_num, name in sorted(web_containers):
            server_targets.append({'label': name, 'value': str(server_num)})

        servers_status = monitoring_data.get('servers_status', [])
        status_map = {s.get('name'): s for s in servers_status}

        return render_template(
            'management.html',
            system_info=monitoring_data.get('host_info', {}),
            containers=containers,
            scripts=list_control_scripts(),
            server_targets=server_targets,
            servers_status=servers_status,
            status_map=status_map,
            last_update=monitoring_data.get('last_update'),
            flash_messages=get_flashed_messages(with_categories=True)
        )

@app.route('/scripts/run', methods=['POST'])
def run_control_script():
    script_name = request.form.get('script_name')
    args = request.form.get('args', '').strip()
    scripts = list_control_scripts()

    if script_name not in scripts:
        flash(f"Not permitted or unknown script: {script_name}", 'error')
        return redirect(url_for('management'))

    arg_list = shlex.split(args) if args else []
    success, output = run_script(script_name, *arg_list)

    if success:
        flash(f"Script {script_name} executed successfully: {output}", 'success')
    else:
        flash(f"Script {script_name} failed: {output}", 'error')

    return redirect(url_for('management'))

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

@app.route('/api/system-info')
def api_system_info():
    """API endpoint for detailed system information"""
    try:
        # CPU information
        cpu_percent = psutil.cpu_percent(interval=1)
        cpu_count = psutil.cpu_count()
        load_avg = psutil.getloadavg() if hasattr(psutil, 'getloadavg') else None

        # Memory information
        memory = psutil.virtual_memory()
        memory_percent = memory.percent
        memory_used = memory.used
        memory_total = memory.total

        # Disk information
        disk = psutil.disk_usage('/')
        disk_used = disk.used
        disk_total = disk.total
        disk_free = disk.free

        # System information
        uptime = None
        try:
            with open('/proc/uptime', 'r') as f:
                uptime_seconds = float(f.readline().split()[0])
                uptime_days = int(uptime_seconds // 86400)
                uptime_hours = int((uptime_seconds % 86400) // 3600)
                uptime_minutes = int((uptime_seconds % 3600) // 60)
                uptime = f"{uptime_days}d {uptime_hours}h {uptime_minutes}m"
        except:
            pass

        process_count = len(psutil.pids())
        os_info = f"{psutil.os.uname().sysname} {psutil.os.uname().release}"

        system_info = {
            'cpu_percent': cpu_percent,
            'cpu_count': cpu_count,
            'load_avg': f"{load_avg[0]:.2f}, {load_avg[1]:.2f}, {load_avg[2]:.2f}" if load_avg else None,
            'memory_percent': memory_percent,
            'memory_used': memory_used,
            'memory_total': memory_total,
            'disk_used': disk_used,
            'disk_total': disk_total,
            'disk_free': disk_free,
            'uptime': uptime,
            'process_count': process_count,
            'os_info': os_info
        }

        return jsonify({
            'system_info': system_info,
            'timestamp': datetime.now().strftime('%Y-%m-%d %H:%M:%S')
        })
    except Exception as e:
        return jsonify({
            'error': str(e),
            'timestamp': datetime.now().strftime('%Y-%m-%d %H:%M:%S')
        }), 500

# Management Routes
@app.route('/servers/start', methods=['POST'])
def start_servers():
    """Start servers"""
    target = request.form.get('target', 'all')
    success, output = run_script('start_servers.sh', target)

    if success:
        flash(f'Servers started successfully: {target}', 'success')
    else:
        flash(f'Failed to start servers: {output}', 'error')

    return redirect(url_for('dashboard'))

@app.route('/servers/stop', methods=['POST'])
def stop_servers():
    """Stop servers without removing container state."""
    target = request.form.get('target', 'all')
    success, output = stop_containers(target)

    if success:
        flash(f'Servers stopped successfully: {target}', 'success')
    else:
        flash(f'Failed to stop servers: {output}', 'error')

    return redirect(url_for('dashboard'))

@app.route('/servers/restart', methods=['POST'])
def restart_servers():
    """Restart servers"""
    target = request.form.get('target', 'all')
    success, output = run_script('restart_servers.sh', target)

    if success:
        flash(f'Servers restarted successfully: {target}', 'success')
    else:
        flash(f'Failed to restart servers: {output}', 'error')

    return redirect(url_for('dashboard'))

@app.route('/servers/add', methods=['POST'])
def add_server():
    """Add new server"""
    try:
        server_num = int(request.form.get('server_num'))
        port = int(request.form.get('port'))

        success, output = run_script('add_server.sh', str(server_num), str(port))

        if success:
            flash(f'Server {server_num} added successfully on port {port}', 'success')
        else:
            flash(f'Failed to add server: {output}', 'error')
    except ValueError:
        flash('Invalid server number or port', 'error')

    return redirect(url_for('dashboard'))

@app.route('/servers/remove', methods=['POST'])
def remove_server():
    """Remove server from load balancer and delete the stopped container."""
    try:
        server_num = int(request.form.get('server_num'))
        port = request.form.get('port')
        if not port:
            port = str(8000 + server_num)

        success, output = run_script('configure_load_balancer.sh', 'remove-server', '127.0.0.1', str(port))
        remove_success, remove_output = run_script('stop_servers.sh', str(server_num))

        if success and remove_success:
            flash(f'Server {server_num} removed successfully', 'success')
        elif success:
            flash(f'Server removed from load balancer, but stopping container failed: {remove_output}', 'warning')
        elif remove_success:
            flash(f'Server container removed, but load balancer update failed: {output}', 'warning')
        else:
            flash(f'Failed to remove server: {output} / {remove_output}', 'error')
    except ValueError:
        flash('Invalid server number or port', 'error')
    except Exception as e:
        flash(f'Error removing server: {str(e)}', 'error')

    return redirect(url_for('dashboard'))

@app.route('/servers/control', methods=['POST'])
def control_server():
    """Control server (start/stop/restart) using shell scripts"""
    target = request.form.get('target') or request.form.get('server_num')
    action = request.form.get('action')

    if not target:
        flash('Server target is required', 'error')
        return redirect(url_for('dashboard'))

    if action not in ['start', 'stop', 'restart']:
        flash('Invalid action. Must be start, stop, or restart.', 'error')
        return redirect(url_for('dashboard'))

    if action == 'start':
        success, output = run_script('start_servers.sh', str(target))
    elif action == 'stop':
        success, output = stop_containers(target)
    else:
        success, output = run_script('restart_servers.sh', str(target))

    if success:
        flash(f'Server target {target} {action}ed successfully', 'success')
    else:
        flash(f'Failed to {action} server target {target}: {output}', 'error')

    return redirect(url_for('dashboard'))

@app.route('/servers/create-docker', methods=['POST'])
def create_docker_server():
    """Create new Docker server using shell script"""
    try:
        server_num = int(request.form.get('server_num'))
        server_port = int(request.form.get('server_port'))

        success, output = run_script('create_docker_server.sh', str(server_num), str(server_port))

        if success:
            return jsonify({
                'success': True,
                'message': f'Server {server_num} created successfully on port {server_port}'
            })
        else:
            return jsonify({
                'success': False,
                'message': f'Failed to create server: {output}'
            })
    except ValueError:
        return jsonify({
            'success': False,
            'message': 'Invalid server number or port'
        })
    except Exception as e:
        return jsonify({
            'success': False,
            'message': f'Error: {str(e)}'
        })

@app.route('/upload', methods=['POST'])
def upload_file():
    """Upload HTML file"""
    if 'file' not in request.files:
        flash('No file selected', 'error')
        return redirect(url_for('dashboard'))

    file = request.files['file']
    target = request.form.get('target', 'all')

    if file.filename == '':
        flash('No file selected', 'error')
        return redirect(url_for('dashboard'))

    if file and allowed_file(file.filename):
        filename = secure_filename(file.filename)
        file_path = os.path.join(app.config['UPLOAD_FOLDER'], filename)
        file.save(file_path)

        success, output = run_script('upload_html.sh', filename, target)

        # Clean up uploaded file
        if os.path.exists(file_path):
            os.remove(file_path)

        if success:
            flash(f'File {filename} uploaded successfully to {target}', 'success')
        else:
            flash(f'Failed to upload file: {output}', 'error')
    else:
        flash('Invalid file type. Only .html files are allowed.', 'error')

    return redirect(url_for('dashboard'))

@app.route('/load-balancer/configure', methods=['POST'])
def configure_lb():
    """Configure load balancer"""
    action = request.form.get('action')

    if action == 'add-server':
        ip = request.form.get('ip')
        port = request.form.get('port')
        success, output = run_script('configure_load_balancer.sh', 'add-server', ip, port)
    elif action == 'remove-server':
        ip = request.form.get('ip')
        port = request.form.get('port')
        success, output = run_script('configure_load_balancer.sh', 'remove-server', ip, port)
    elif action == 'set-method':
        method = request.form.get('method')
        success, output = run_script('configure_load_balancer.sh', 'set-method', method)
    else:
        flash('Invalid action', 'error')
        return redirect(url_for('dashboard'))

    if success:
        flash(f'Load balancer configured successfully', 'success')
    else:
        flash(f'Failed to configure load balancer: {output}', 'error')

    return redirect(url_for('dashboard'))

@app.route('/load-balancer/update', methods=['POST'])
def update_lb():
    """Update load balancer with all running servers"""
    success, output = run_script('update_load_balancer.sh')

    if success:
        flash('Load balancer updated successfully with all running servers', 'success')
    else:
        flash(f'Failed to update load balancer: {output}', 'error')

    return redirect(url_for('dashboard'))

@app.route('/monitoring/hosts', methods=['POST'])
def monitor_hosts():
    """Monitor hosts"""
    host_ip = request.form.get('host_ip', '127.0.0.1')
    action = request.form.get('action', 'all')

    success, output = run_script('monitor_hosts.sh', host_ip, action)

    return jsonify({
        'success': success,
        'output': output,
        'host': host_ip,
        'action': action
    })

@app.route('/ssh/login', methods=['POST'])
def ssh_login():
    """SSH login (returns connection info, actual SSH handled by script)"""
    host_ip = request.form.get('host_ip')
    username = request.form.get('username', 'root')
    port = request.form.get('port', '22')

    if not host_ip:
        return jsonify({'success': False, 'error': 'Host IP is required'})

    # For web interface, we'll return connection info
    # Actual SSH connection would need to be handled differently for web
    return jsonify({
        'success': True,
        'message': f'SSH connection info prepared for {username}@{host_ip}:{port}',
        'command': f'ssh -p {port} {username}@{host_ip}'
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
