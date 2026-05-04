import os
import json
import socket
import psutil
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

# Configuration - dynamic paths
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
PROJECT_DIR = os.path.abspath(os.path.join(SCRIPT_DIR, ".."))
CONTROL_SCRIPTS_DIR = os.path.join(PROJECT_DIR, "control-scripts")
UPLOAD_FOLDER = os.path.join(PROJECT_DIR, "uploads")
LAN_CONFIG_FILE = os.path.join(PROJECT_DIR, "lan_config.json")
ALLOWED_EXTENSIONS = {'html'}
DOCKER_NETWORK = "p1-3server_lb_network"

app.config['UPLOAD_FOLDER'] = UPLOAD_FOLDER
os.makedirs(UPLOAD_FOLDER, exist_ok=True)

# Docker client
docker_available = False
use_subprocess = True
client = None

try:
    result = subprocess.run(['docker', 'version'], capture_output=True, text=True, timeout=5)
    if result.returncode == 0:
        try:
            import docker
            client = docker.from_env()
            client.ping()
            docker_available = True
            use_subprocess = False
            print("✅ Docker Python library connected")
        except:
            docker_available = True
            use_subprocess = True
            print("⚠️ Using subprocess fallback for Docker")
    else:
        raise Exception("Docker not accessible")
except Exception as e:
    print(f"⚠️ Docker: {e}")
    docker_available = True
    use_subprocess = True

# Monitoring data store
monitoring_data = {
    'containers': [], 'host_info': {}, 'servers_status': [],
    'request_counts': {}, 'last_update': None
}
data_lock = threading.Lock()

# ============ LAN Config ============
def load_lan_config():
    try:
        with open(LAN_CONFIG_FILE, 'r') as f:
            return json.load(f)
    except:
        default = {
            "networks": [
                {"id": "admin_lan", "name": "Admin LAN", "port": 8080, "role": "admin",
                 "description": "To'liq boshqaruv", "created_at": datetime.now().isoformat(),
                 "allowed_ips": ["*"], "active": True},
                {"id": "user_lan", "name": "User LAN", "port": 8082, "role": "user",
                 "description": "Standart foydalanish", "created_at": datetime.now().isoformat(),
                 "allowed_ips": ["*"], "active": True},
                {"id": "reader_lan", "name": "Reader LAN", "port": 8081, "role": "reader",
                 "description": "Faqat ko'rish", "created_at": datetime.now().isoformat(),
                 "allowed_ips": ["*"], "active": True}
            ],
            "roles": {
                "admin": {"label": "Admin", "color": "#ef4444",
                          "permissions": ["read", "write", "execute", "manage_servers", "manage_lan", "configure_lb"]},
                "user": {"label": "User", "color": "#3b82f6", "permissions": ["read", "write"]},
                "reader": {"label": "Reader", "color": "#22c55e", "permissions": ["read"]}
            }
        }
        save_lan_config(default)
        return default

def save_lan_config(config):
    with open(LAN_CONFIG_FILE, 'w') as f:
        json.dump(config, f, indent=2, ensure_ascii=False)

# ============ Helpers ============
def get_local_ip():
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.connect(("8.8.8.8", 80))
        ip = s.getsockname()[0]
        s.close()
        return ip
    except:
        return "127.0.0.1"

def get_container_info():
    if not docker_available:
        return []
    if use_subprocess:
        try:
            result = subprocess.run(['docker', 'ps', '-a', '--format', '{{.Names}}|||{{.Status}}|||{{.ID}}|||{{.Image}}|||{{.Ports}}'],
                                    capture_output=True, text=True, timeout=10)
            if result.returncode != 0:
                return []
            containers_info = []
            for line in result.stdout.strip().split('\n'):
                if not line.strip():
                    continue
                parts = line.split('|||')
                if len(parts) < 5:
                    continue
                name, status, cid, image, ports = parts[0].replace('/', ''), parts[1], parts[2][:12], parts[3], parts[4]
                if any(n in name for n in ['load_balancer', 'web_server']):
                    containers_info.append({
                        'name': name,
                        'id': cid,
                        'status': status,
                        'state': 'running' in status.lower() or 'up' in status.lower(),
                        'image': image,
                        'ports': ports,
                        'cpu_percent': 'N/A', 'memory_mb': 'N/A'
                    })
            return containers_info
        except Exception as e:
            print(f"Error: {e}")
            return []
    else:
        containers_info = []
        try:
            for container in client.containers.list(all=True):
                if any(n in container.name for n in ['load_balancer', 'web_server']):
                    containers_info.append({
                        'name': container.name,
                        'id': container.id[:12],
                        'status': container.status,
                        'state': 'running' in container.status.lower(),
                        'image': container.image.tags[0] if container.image.tags else 'unknown',
                        'ports': str(container.attrs.get('NetworkSettings', {}).get('Ports', '')),
                        'cpu_percent': 'N/A', 'memory_mb': 'N/A'
                    })
        except Exception as e:
            print(f"Error: {e}")
        return containers_info

def check_server_health():
    servers = []
    if docker_available:
        try:
            if use_subprocess:
                result = subprocess.run(['docker', 'ps', '-a', '--filter', 'name=web_server_', '--format', '{{.Names}}|||{{.Status}}'],
                                        capture_output=True, text=True, timeout=5)
                if result.returncode == 0:
                    for line in result.stdout.strip().split('\n'):
                        if not line.strip():
                            continue
                        try:
                            parts = line.split('|||')
                            if len(parts) < 2: continue
                            name = parts[0].replace('/', '')
                            num = int(name.split('_')[-1])
                            port = 8000 + num
                            servers.append({'name': name, 'port': port,
                                            'url': f'http://127.0.0.1:{port}',
                                            'status_text': parts[1]})
                        except:
                            continue
            else:
                for container in client.containers.list(all=True, filters={'name': 'web_server_'}):
                    try:
                        num = int(container.name.split('_')[-1])
                        port = 8000 + num
                        servers.append({'name': container.name, 'port': port,
                                        'url': f'http://127.0.0.1:{port}',
                                        'status_text': container.status})
                    except:
                        continue
        except:
            pass

    if not servers:
        servers = [{'name': f'web_server_{i}', 'port': 8000+i,
                     'url': f'http://127.0.0.1:{8000+i}', 'status_text': 'unknown'} for i in range(1, 4)]

    result = []
    for srv in servers:
        st = srv.get('status_text', '')
        if st and 'up' not in st.lower():
            result.append({'name': srv['name'], 'port': srv['port'], 'url': srv['url'],
                           'status': 'STOPPED' if 'exited' in st.lower() else 'DOWN',
                           'response_time_ms': 'N/A', 'status_code': 0, 'healthy': False, 'status_text': st})
        else:
            try:
                start = time.time()
                resp = requests.get(srv['url'], timeout=2)
                rt = (time.time() - start) * 1000
                result.append({'name': srv['name'], 'port': srv['port'], 'url': srv['url'],
                               'status': 'UP' if resp.status_code < 500 else 'ERROR',
                               'response_time_ms': f"{rt:.2f}", 'status_code': resp.status_code,
                               'healthy': resp.status_code < 500, 'status_text': st})
            except requests.exceptions.Timeout:
                result.append({'name': srv['name'], 'port': srv['port'], 'url': srv['url'],
                               'status': 'TIMEOUT', 'response_time_ms': '>2000',
                               'status_code': 0, 'healthy': False, 'status_text': st})
            except:
                result.append({'name': srv['name'], 'port': srv['port'], 'url': srv['url'],
                               'status': 'DOWN', 'response_time_ms': 'N/A',
                               'status_code': 0, 'healthy': False, 'status_text': st})
    return result

def check_load_balancer():
    try:
        start = time.time()
        resp = requests.get('http://127.0.0.1:8082', timeout=2)
        rt = (time.time() - start) * 1000
        return {'name': 'load_balancer', 'port': 8082, 'url': 'http://127.0.0.1:8082',
                'status': 'UP' if resp.status_code < 500 else 'ERROR',
                'response_time_ms': f"{rt:.2f}", 'status_code': resp.status_code,
                'healthy': resp.status_code < 500}
    except:
        return {'name': 'load_balancer', 'port': 8082, 'url': 'http://127.0.0.1:8082',
                'status': 'DOWN', 'response_time_ms': 'N/A', 'status_code': 0, 'healthy': False}

def get_host_info():
    try:
        cpu = psutil.cpu_percent(interval=1)
        mem = psutil.virtual_memory()
        return {
            'hostname': socket.gethostname(), 'local_ip': get_local_ip(),
            'cpu_percent': f"{cpu:.1f}%", 'memory_percent': f"{mem.percent:.1f}%",
            'memory_mb': f"{mem.used / (1024**2):.0f}",
            'memory_total_mb': f"{mem.total / (1024**2):.0f}",
            'disk_percent': f"{psutil.disk_usage('/').percent:.1f}%"
        }
    except:
        return {}

def allowed_file(filename):
    return '.' in filename and filename.rsplit('.', 1)[1].lower() in ALLOWED_EXTENSIONS

def run_script(script_name, *args):
    script_path = os.path.join(CONTROL_SCRIPTS_DIR, script_name)
    if not os.path.exists(script_path):
        return False, f"Script {script_name} not found"
    try:
        cmd = [script_path] + list(args)
        result = subprocess.run(cmd, capture_output=True, text=True, cwd=PROJECT_DIR, timeout=60)
        return result.returncode == 0, (result.stdout if result.returncode == 0 else result.stderr).strip()
    except subprocess.TimeoutExpired:
        return False, "Script timed out"
    except Exception as e:
        return False, str(e)

def docker_stop_container(name):
    if not docker_available:
        return False, "Docker not available"
    try:
        if use_subprocess:
            r1 = subprocess.run(['docker', 'stop', name], capture_output=True, text=True, timeout=20)
            r2 = subprocess.run(['docker', 'rm', name], capture_output=True, text=True, timeout=20)
            return r1.returncode == 0 or r2.returncode == 0, f"Stopped/Removed {name}"
        else:
            container = client.containers.get(name)
            container.stop()
            container.remove()
            return True, f"Stopped and removed {name}"
    except Exception as e:
        return 'not running' in str(e).lower(), str(e)

def stop_containers(target):
    target = str(target)
    names = []
    if target == 'all':
        names.append('load_balancer')
        try:
            if use_subprocess:
                r = subprocess.run(['docker', 'ps', '-a', '--filter', 'name=web_server_', '--format', '{{.Names}}'],
                                   capture_output=True, text=True, timeout=10)
                if r.returncode == 0:
                    names.extend([n.strip() for n in r.stdout.split('\n') if n.strip()])
            else:
                names.extend([c.name for c in client.containers.list(all=True, filters={'name': 'web_server_'})])
        except Exception as e:
            return False, str(e)
    elif target == 'lb':
        names = ['load_balancer']
    elif target.isdigit():
        names = [f'web_server_{target}']
    else:
        return False, 'Invalid target'
    msgs = []
    for n in names:
        ok, out = docker_stop_container(n)
        msgs.append(f"{n}: {out}")
        if not ok:
            return False, '; '.join(msgs)
    return True, '; '.join(msgs)

def list_control_scripts():
    try:
        return sorted([f for f in os.listdir(CONTROL_SCRIPTS_DIR)
                       if os.path.isfile(os.path.join(CONTROL_SCRIPTS_DIR, f)) and f.endswith('.sh')])
    except:
        return []

def update_monitoring_data():
    while True:
        try:
            with data_lock:
                monitoring_data['containers'] = get_container_info()
                monitoring_data['host_info'] = get_host_info()
                monitoring_data['servers_status'] = check_server_health()
                monitoring_data['load_balancer'] = check_load_balancer()
                monitoring_data['last_update'] = datetime.now().strftime('%Y-%m-%d %H:%M:%S')
        except Exception as e:
            print(f"Monitor error: {e}")
        time.sleep(3)

# ============ Routes ============
@app.route('/')
def dashboard():
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
        server_targets = [{'label': 'Barcha serverlar', 'value': 'all'},
                          {'label': 'Load Balancer', 'value': 'lb'}]
        web_containers = []
        for c in containers:
            name = c.get('name', '')
            if name.startswith('web_server_'):
                try:
                    web_containers.append((int(name.rsplit('_', 1)[-1]), name))
                except ValueError:
                    continue
        for num, name in sorted(web_containers):
            server_targets.append({'label': name, 'value': str(num)})
        servers_status = monitoring_data.get('servers_status', [])
        status_map = {s.get('name'): s for s in servers_status}
        return render_template('management.html',
            system_info=monitoring_data.get('host_info', {}),
            containers=containers, scripts=list_control_scripts(),
            server_targets=server_targets, servers_status=servers_status,
            status_map=status_map, last_update=monitoring_data.get('last_update'),
            flash_messages=get_flashed_messages(with_categories=True))

@app.route('/view/lan')
def lan_management():
    config = load_lan_config()
    return render_template('lan.html', lan_config=config,
                           last_update=monitoring_data.get('last_update'))

# ============ API Routes ============
@app.route('/api/status')
def api_status():
    with data_lock:
        return jsonify(monitoring_data)

@app.route('/api/containers')
def api_containers():
    with data_lock:
        return jsonify({'containers': monitoring_data['containers'],
                        'timestamp': monitoring_data['last_update']})

@app.route('/api/servers')
def api_servers():
    with data_lock:
        return jsonify({'load_balancer': monitoring_data.get('load_balancer', {}),
                        'backends': monitoring_data['servers_status'],
                        'timestamp': monitoring_data['last_update']})

@app.route('/api/host')
def api_host():
    with data_lock:
        return jsonify({'host': monitoring_data['host_info'],
                        'timestamp': monitoring_data['last_update']})

@app.route('/api/system-info')
def api_system_info():
    try:
        cpu = psutil.cpu_percent(interval=1)
        mem = psutil.virtual_memory()
        disk = psutil.disk_usage('/')
        uptime = None
        try:
            with open('/proc/uptime', 'r') as f:
                s = float(f.readline().split()[0])
                uptime = f"{int(s//86400)}d {int((s%86400)//3600)}h {int((s%3600)//60)}m"
        except:
            pass
        return jsonify({'system_info': {
            'cpu_percent': cpu, 'cpu_count': psutil.cpu_count(),
            'memory_percent': mem.percent, 'memory_used': mem.used, 'memory_total': mem.total,
            'disk_used': disk.used, 'disk_total': disk.total, 'disk_free': disk.free,
            'uptime': uptime, 'process_count': len(psutil.pids())
        }, 'timestamp': datetime.now().strftime('%Y-%m-%d %H:%M:%S')})
    except Exception as e:
        return jsonify({'error': str(e)}), 500

# ============ Metrics & Active Servers API ============
import random

@app.route('/api/metrics/server/<name>')
def api_metrics_server(name):
    return jsonify({
        'name': name,
        'cpu': [random.randint(5, 75) for _ in range(10)],
        'memory': [random.randint(10, 50) for _ in range(10)],
        'requests_per_sec': random.randint(10, 200),
        'active_users': random.randint(1, 50),
        'labels': [datetime.now().strftime('%H:%M:%S') for _ in range(10)]
    })

@app.route('/api/metrics/lan/<lan_id>')
def api_metrics_lan(lan_id):
    return jsonify({
        'id': lan_id,
        'bandwidth': [random.randint(5, 50) for _ in range(10)],
        'active_pcs': random.randint(1, 20),
        'blocked_requests': random.randint(0, 5) if lan_id == 'reader_lan' else 0,
        'labels': [datetime.now().strftime('%H:%M:%S') for _ in range(10)]
    })

@app.route('/api/scripts')
def api_scripts():
    return jsonify({'scripts': list_control_scripts()})

@app.route('/api/active-servers')
def api_active_servers():
    with data_lock:
        containers = monitoring_data.get('containers', [])
        web_containers = []
        for c in containers:
            name = c.get('name', '')
            if name.startswith('web_server_'):
                try:
                    web_containers.append({'num': int(name.rsplit('_', 1)[-1]), 'name': name})
                except ValueError:
                    pass
        return jsonify({'servers': sorted(web_containers, key=lambda x: x['num'])})

# ============ LAN API Routes ============
@app.route('/api/lan/networks')
def api_lan_networks():
    return jsonify(load_lan_config())

@app.route('/api/lan/create', methods=['POST'])
def api_lan_create():
    data = request.get_json() or request.form
    name = data.get('name', '')
    port = data.get('port', '')
    role = data.get('role', 'user')
    desc = data.get('description', '')
    if not name or not port:
        return jsonify({'success': False, 'message': 'Name and port required'}), 400
    config = load_lan_config()
    lan_id = name.lower().replace(' ', '_')
    for n in config['networks']:
        if n['id'] == lan_id or str(n['port']) == str(port):
            return jsonify({'success': False, 'message': 'Network or port already exists'}), 400
    config['networks'].append({
        'id': lan_id, 'name': name, 'port': int(port), 'role': role,
        'description': desc, 'created_at': datetime.now().isoformat(),
        'allowed_ips': ['*'], 'active': True
    })
    save_lan_config(config)
    return jsonify({'success': True, 'message': f'LAN "{name}" created on port {port}'})

@app.route('/api/lan/delete', methods=['POST'])
def api_lan_delete():
    data = request.get_json() or request.form
    lan_id = data.get('id', '')
    if lan_id in ['admin_lan', 'user_lan', 'reader_lan']:
        return jsonify({'success': False, 'message': 'Default networks cannot be deleted'}), 400
    config = load_lan_config()
    config['networks'] = [n for n in config['networks'] if n['id'] != lan_id]
    save_lan_config(config)
    return jsonify({'success': True, 'message': f'LAN "{lan_id}" deleted'})

@app.route('/api/lan/update-role', methods=['POST'])
def api_lan_update_role():
    data = request.get_json() or request.form
    lan_id = data.get('id', '')
    new_role = data.get('role', '')
    if new_role not in ['admin', 'user', 'reader']:
        return jsonify({'success': False, 'message': 'Invalid role'}), 400
    config = load_lan_config()
    for n in config['networks']:
        if n['id'] == lan_id:
            n['role'] = new_role
            save_lan_config(config)
            return jsonify({'success': True, 'message': f'Role updated to {new_role}'})
    return jsonify({'success': False, 'message': 'Network not found'}), 404

@app.route('/api/lan/toggle', methods=['POST'])
def api_lan_toggle():
    data = request.get_json() or request.form
    lan_id = data.get('id', '')
    config = load_lan_config()
    for n in config['networks']:
        if n['id'] == lan_id:
            n['active'] = not n.get('active', True)
            save_lan_config(config)
            return jsonify({'success': True, 'active': n['active']})
    return jsonify({'success': False, 'message': 'Not found'}), 404

# ============ Server Control Routes ============
@app.route('/scripts/run', methods=['POST'])
def run_control_script():
    script_name = request.form.get('script_name')
    args = request.form.get('args', '').strip()
    if script_name not in list_control_scripts():
        flash(f"Unknown script: {script_name}", 'error')
        return redirect(url_for('management'))
    arg_list = shlex.split(args) if args else []
    ok, out = run_script(script_name, *arg_list)
    flash(f"{'✓' if ok else '✗'} {script_name}: {out}", 'success' if ok else 'error')
    return redirect(url_for('management'))

@app.route('/servers/start', methods=['POST'])
def start_servers():
    target = request.form.get('target', 'all')
    ok, out = run_script('start_servers.sh', target)
    flash(f"{'Started' if ok else 'Failed'}: {out}", 'success' if ok else 'error')
    return redirect(url_for('management'))

@app.route('/servers/stop', methods=['POST'])
def stop_servers():
    target = request.form.get('target', 'all')
    ok, out = stop_containers(target)
    flash(f"{'Stopped' if ok else 'Failed'}: {out}", 'success' if ok else 'error')
    return redirect(url_for('management'))

@app.route('/servers/restart', methods=['POST'])
def restart_servers():
    target = request.form.get('target', 'all')
    ok, out = run_script('restart_servers.sh', target)
    flash(f"{'Restarted' if ok else 'Failed'}: {out}", 'success' if ok else 'error')
    return redirect(url_for('management'))

@app.route('/servers/add', methods=['POST'])
def add_server():
    try:
        num = int(request.form.get('server_num'))
        port = int(request.form.get('port'))
        ok, out = run_script('add_server.sh', str(num), str(port))
        flash(f"{'Added' if ok else 'Failed'}: {out}", 'success' if ok else 'error')
    except ValueError:
        flash('Invalid server number or port', 'error')
    return redirect(url_for('management'))

@app.route('/servers/remove', methods=['POST'])
def remove_server():
    try:
        num = int(request.form.get('server_num'))
        port = request.form.get('port', str(8000 + num))
        ok1, o1 = run_script('configure_load_balancer.sh', 'remove-server', f'web_server_{num}', '80')
        ok2, o2 = stop_containers(str(num))
        if ok1 and ok2:
            flash(f'Server {num} removed', 'success')
        else:
            flash(f'Partial: LB={o1}, Stop={o2}', 'warning')
    except Exception as e:
        flash(str(e), 'error')
    return redirect(url_for('management'))

@app.route('/servers/control', methods=['POST'])
def control_server():
    target = request.form.get('target') or request.form.get('server_num')
    action = request.form.get('action')
    if not target or action not in ['start', 'stop', 'restart']:
        flash('Invalid request', 'error')
        return redirect(url_for('management'))
    if action == 'stop':
        ok, out = stop_containers(target)
    else:
        ok, out = run_script(f'{action}_servers.sh', str(target))
    flash(f"{'✓' if ok else '✗'} {action} {target}: {out}", 'success' if ok else 'error')
    return redirect(url_for('management'))

@app.route('/servers/create-docker', methods=['POST'])
def create_docker_server():
    try:
        num = int(request.form.get('server_num'))
        port = int(request.form.get('server_port'))
        ok, out = run_script('create_docker_server.sh', str(num), str(port))
        return jsonify({'success': ok, 'message': out})
    except Exception as e:
        return jsonify({'success': False, 'message': str(e)})

@app.route('/upload', methods=['POST'])
def upload_file():
    if 'file' not in request.files:
        flash('No file selected', 'error')
        return redirect(url_for('management'))
    file = request.files['file']
    target = request.form.get('target', 'all')
    if file.filename == '' or not allowed_file(file.filename):
        flash('Invalid file', 'error')
        return redirect(url_for('management'))
    filename = secure_filename(file.filename)
    path = os.path.join(app.config['UPLOAD_FOLDER'], filename)
    file.save(path)
    ok, out = run_script('upload_html.sh', filename, target)
    if os.path.exists(path):
        os.remove(path)
    flash(f"{'Uploaded' if ok else 'Failed'}: {out}", 'success' if ok else 'error')
    return redirect(url_for('management'))

@app.route('/load-balancer/configure', methods=['POST'])
def configure_lb():
    action = request.form.get('action')
    if action == 'add-server':
        ok, out = run_script('configure_load_balancer.sh', 'add-server',
                             request.form.get('ip'), request.form.get('port'))
    elif action == 'remove-server':
        ok, out = run_script('configure_load_balancer.sh', 'remove-server',
                             request.form.get('ip'), request.form.get('port'))
    elif action == 'set-method':
        ok, out = run_script('configure_load_balancer.sh', 'set-method', request.form.get('method'))
    else:
        flash('Invalid action', 'error')
        return redirect(url_for('management'))
    flash(f"{'✓' if ok else '✗'} LB config: {out}", 'success' if ok else 'error')
    return redirect(url_for('management'))

@app.route('/load-balancer/update', methods=['POST'])
def update_lb():
    ok, out = run_script('update_load_balancer.sh')
    flash(f"{'Updated' if ok else 'Failed'}: {out}", 'success' if ok else 'error')
    return redirect(url_for('management'))

if __name__ == '__main__':
    monitor_thread = threading.Thread(target=update_monitoring_data, daemon=True)
    monitor_thread.start()
    time.sleep(2)
    print(f"🎯 Dashboard: http://localhost:5000")
    print(f"🔗 Local IP: {get_local_ip()}")
    app.run(host='0.0.0.0', port=5000, debug=False)
