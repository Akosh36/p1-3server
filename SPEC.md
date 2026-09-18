# p1-3server — Setup Specification

This document describes the p1-3server system as it exists on the `master`
branch: a self-hosted, Docker-based, highly-available web server demo for a
home LAN, plus a Flask dashboard for monitoring and controlling it.

Statements below are drawn directly from the repository's config and scripts.
Anywhere the source files were ambiguous, silent, or contradicted each other,
this is marked **(Inferred)** or **(Observed inconsistency)**.

## 1. Architecture overview

```
                 LAN clients
                      │
        ┌─────────────┼─────────────┐
        │ :8080        │ :8081        │ :8082
        ▼ admin         ▼ reader       ▼ user
┌───────────────────────────────────────────────┐
│         load_balancer (nginx:alpine)           │
│   - 8080/8081 → Flask dashboard (172.20.0.1:5000)
│   - 8082      → round-robin proxy to `backend` │
└───────────────┬─────────────────────────────────┘
                │ upstream "backend"
        ┌───────┼────────┬────────┐
        ▼        ▼        ▼        ▼
   web_server_2 web_server_3 web_server_4 web_server_5
   (nginx:alpine, each serving the same index.html)
```

- **Load balancer**: one `nginx:alpine` container, named `load_balancer`,
  configured by `nginx.conf`. It is the single entry point and reverse
  proxy for the whole system.
- **Backends**: `nginx:alpine` containers named `web_server_<N>`, each
  serving a shared `index.html`. `docker-compose.yml` defines
  `web_server_1` through `web_server_5`.
- **Network**: a dedicated Docker bridge network on subnet `172.20.0.0/16`.
  The load balancer is pinned to `172.20.0.2`; backends are pinned to
  `172.20.0.1<N>` (e.g. `web_server_1` → `172.20.0.11`). In
  `docker-compose.yml` the network is named `lb_network`; the
  `control-scripts/` (which start containers with `docker run` instead of
  Compose) create/use a network literally named `p1-3server_lb_network`
  instead. **(Observed inconsistency)** — the two approaches don't share a
  network name, so containers started by one method won't reach containers
  started by the other.
- **Dashboard**: a Flask app (`view/monitor.py`) run directly on the Docker
  host (not containerized), listening on port `5000`. The load balancer's
  admin and reader tiers reverse-proxy to it at `172.20.0.1:5000`, i.e. the
  Docker bridge gateway IP on the host. **(Inferred)** — `172.20.0.1` is
  assumed to be the host's address on the `lb_network`/`p1-3server_lb_network`
  bridge, which is the Docker default gateway for a `172.20.0.0/16` bridge
  network.

## 2. Load balancer and backend containers

### `docker-compose.yml`

Defines the declarative topology:

| Service | Image | Host port | Container IP |
|---|---|---|---|
| `web_server_1` | nginx:alpine | 8001:80 | 172.20.0.11 |
| `web_server_2` | nginx:alpine | 8002:80 | 172.20.0.12 |
| `web_server_3` | nginx:alpine | 8003:80 | 172.20.0.13 |
| `web_server_4` | nginx:alpine | 8004:80 | 172.20.0.14 |
| `web_server_5` | nginx:alpine | 8005:80 | 172.20.0.15 |
| `load_balancer` | nginx:alpine | 8080:8080, 8081:8081, 8082:8082 | 172.20.0.2 |

Every backend mounts the same `./index.html` read-only into
`/usr/share/nginx/html/index.html`. The load balancer mounts `./nginx.conf`
read-only into `/etc/nginx/nginx.conf` and `depends_on` all five backends.
All services use `restart: unless-stopped`.

### `nginx.conf` (load balancer configuration)

The `upstream backend` block currently lists only four of the five
Compose backends:

```nginx
upstream backend {
    server web_server_4:80 max_fails=3 fail_timeout=10s;
    server web_server_5:80 max_fails=3 fail_timeout=10s;
    server web_server_2:80 max_fails=3 fail_timeout=10s;
    server web_server_3:80 max_fails=3 fail_timeout=10s;
    keepalive 32;
}
```

**(Observed inconsistency)** — `web_server_1` is defined in
`docker-compose.yml` but is absent from this upstream block, so it never
receives traffic even when running.

## 3. Round-robin and failover settings

- **Algorithm**: plain nginx round-robin (the default — no `least_conn` or
  `ip_hash` directive is present). `control-scripts/configure_load_balancer.sh
  set-method <round_robin|least_conn|ip_hash>` can switch this by inserting or
  removing a directive at the top of the `upstream backend` block.
- **Passive health checks**: each upstream server has
  `max_fails=3 fail_timeout=10s` — after 3 failed proxy attempts nginx marks
  that backend down for 10 seconds before retrying it. This is nginx's
  built-in passive upstream health check; there is no active
  `health_check` directive (that requires nginx Plus).
- **Connection reuse**: `keepalive 32` on the upstream block, paired with
  `proxy_http_version 1.1` and `proxy_set_header Connection ""` in the
  `location /` blocks that proxy to it.
- **Timeouts**: `proxy_connect_timeout 5s`, `proxy_send_timeout 60s`,
  `proxy_read_timeout 60s` on proxied locations.
- **Rate limiting**: a `limit_req_zone` (`general`, 10 MB, 100 requests/sec
  per client IP) is defined and applied only on the user-tier server block
  (`limit_req zone=general burst=200 nodelay;`).
- Dynamically adding/removing a backend from the pool (`add_server.sh`,
  `create_docker_server.sh`, `configure_load_balancer.sh add-server`) inserts
  the same `max_fails=3 fail_timeout=10s` line and then either reloads nginx
  (`docker exec load_balancer nginx -s reload`) or, if that fails, restarts
  the `load_balancer` container.

## 4. Access tiers (`lan_config.json` and nginx server blocks)

`lan_config.json` declares three named LAN "networks", each with a role and
a set of permissions:

| Network id | Port | Role | Permissions |
|---|---|---|---|
| `admin_lan` | 8080 | `admin` | read, write, execute, manage_servers, manage_lan, configure_lb |
| `reader_lan` | 8081 | `reader` | read |
| `user_lan` | 8082 | `user` | read, write |

The actual enforcement of these tiers lives in `nginx.conf`, as three
separate `server` blocks listening on the three ports:

- **User LAN — port 8082 (`default_server`)**: proxies `location /` to the
  `upstream backend` pool (the load-balanced nginx backends). Rate-limited
  as described above. Has a `/health` endpoint and blocks dotfile access.
- **Admin LAN — port 8080**: proxies `location /` to the Flask dashboard at
  `172.20.0.1:5000` with no method restriction — full control surface.
- **Reader LAN — port 8081**: proxies to the same Flask dashboard, but an
  `if ($request_method !~ ^(GET|HEAD)$)` guard returns `403 Forbidden` for
  any non-GET/HEAD request, enforcing the reader's read-only role at the
  proxy layer.

`lan_config.json` itself is read/written by `view/monitor.py`
(`load_lan_config`/`save_lan_config`) and served through the dashboard's
`/view/lan` page and `/api/lan/*` endpoints (`networks`, `create`, `delete`,
`update-role`, `toggle`). **(Inferred)** — these dashboard endpoints let an
admin register additional named "LAN" entries (e.g. a 4th network on a new
port) and edit roles/permissions in `lan_config.json`, but nothing in the
codebase wires a new entry back into `nginx.conf` automatically: adding a
new tier here only changes the JSON/dashboard bookkeeping, not the running
load balancer's actual port bindings, which still come from the three
hardcoded `server` blocks above.

## 5. Flask monitoring dashboard (`view/monitor.py`)

- A single-file Flask app, run standalone (not in Docker) with
  `app.run(host='0.0.0.0', port=5000)`.
- On startup it spawns a daemon thread (`update_monitoring_data`) that polls
  every 3 seconds: Docker container list/status, host CPU/memory/disk
  (via `psutil`), backend health (HTTP GET to each `web_server_*` on its
  `800<N>` port), and load-balancer health (HTTP GET to
  `http://127.0.0.1:8082`). Results are cached in an in-process
  `monitoring_data` dict guarded by a lock.
- Docker access is via the `docker` Python SDK if importable and the
  daemon is reachable; otherwise it falls back to shelling out to the
  `docker` CLI (`use_subprocess = True`).
- **Pages**: `/` and `/view/dashboard` (metrics/status view), `/view/management`
  (server start/stop/restart/add/remove, script runner, file upload, load
  balancer configuration), `/view/lan` (the LAN/RBAC table from
  `lan_config.json`).
- **Read-only APIs**: `/api/status`, `/api/containers`, `/api/servers`,
  `/api/host`, `/api/system-info`, `/api/scripts`, `/api/active-servers`,
  `/api/lan/networks`. `/api/metrics/server/<name>` and
  `/api/metrics/lan/<lan_id>` return **randomly generated placeholder data**
  (`random.randint(...)`), not real measurements — **(Observed)** these are
  not wired to real metrics sources.
- **Mutating APIs/routes**: `/api/lan/create`, `/api/lan/delete`,
  `/api/lan/update-role`, `/api/lan/toggle` (edit `lan_config.json`);
  `/scripts/run` (run any script from `control-scripts/` by name with
  arguments), `/servers/start`, `/servers/stop`, `/servers/restart`,
  `/servers/add`, `/servers/remove`, `/servers/control`,
  `/servers/create-docker`, `/upload` (HTML file upload, restricted to
  `.html`), `/load-balancer/configure`, `/load-balancer/update`.
- All of these mutating routes ultimately shell out to scripts in
  `control-scripts/` (see below) via a `run_script()` helper, or call the
  `docker` CLI/SDK directly for stop/remove operations.
- No authentication is implemented on any dashboard route.
  **(Observed)** — access control for the dashboard relies entirely on
  which nginx tier (8080 vs 8081) a client's request comes in on; the
  Flask process itself, and port 5000 directly, has no login and no
  request-method restriction.

## 6. Control scripts (`control-scripts/`)

Two generations of scripts coexist in the repo:

### Current (bridge-network, dynamic; what the dashboard calls)

| Script | Purpose |
|---|---|
| `start_servers.sh [<N>\|all\|lb]` | Creates the `p1-3server_lb_network` bridge if missing, then `docker run`s backend `web_server_<N>` (port `800N`, IP `172.20.0.1<N>`) and/or `load_balancer` (ports 8080/8081/8082, IP `172.20.0.2`), each with `--restart unless-stopped`. |
| `stop_servers.sh [<N>\|all\|lb]` | `docker stop`/`docker rm` the matching container(s). |
| `restart_servers.sh [<N>\|all\|lb]` | Calls `stop_servers.sh` then `start_servers.sh` for the target. |
| `add_server.sh <N> <port>` | Patches `docker-compose.yml` (inserts a new `web_server_<N>` service before `load_balancer`, adds it to `depends_on`) and `nginx.conf` (adds it to the `upstream backend` block) via `sed`. Does not itself start the container. |
| `create_docker_server.sh <N> <port>` | Writes a new per-server `nginx_<port>.conf`, `docker run`s `web_server_<N>` on the bridge network with that config mounted, adds it to `nginx.conf`'s upstream block, and reloads the load balancer if running. |
| `upload_html.sh <file.html> [<N>\|all]` | Copies an HTML file into `index.html` (used by all backends via the shared bind mount) and/or `docker cp`s it directly into one running container. |
| `configure_load_balancer.sh <list\|add-server\|remove-server\|set-method>` | Edits the `upstream backend` block or load-balancing method directive in `nginx.conf` via `sed`, then reloads/restarts `load_balancer`. |
| `update_load_balancer.sh` | Rebuilds the entire `upstream backend` block from whatever `web_server_*` containers are currently running (via `docker ps`), then reloads/restarts the load balancer. |
| `monitor_hosts.sh <ip> [ping\|ssh-check\|services\|network\|ports\|processes\|system\|all]` | Ad-hoc host diagnostics (ping, port checks including 8080/8081/8082/8001-8003, `docker ps`, local system stats); for a non-local `ip`, most actions require SSH. |
| `ssh_login.sh <ip> [user] [port]` | Checks host reachability and SSH port, then opens an interactive `ssh` session. |

### Legacy (host-network, static 3-server layout)

`start.sh`, `stop.sh`, `start1.sh`/`start2.sh`/`start3.sh`,
`stop1.sh`/`stop2.sh`/`stop3.sh`. These `cd` into a hardcoded path
(`/home/user/Documents/Projects/p1-3server`), run containers with
`--network host` instead of the bridge network, and mount per-port config
files (`nginx_8001.conf`, etc.) that expect the load balancer on port 80.
**(Observed inconsistency)** — this layout predates (or is unrelated to)
the current `docker-compose.yml`/`nginx.conf` (which use a bridge network,
5 backends, and ports 8080/8081/8082), and the hardcoded path will not
exist on a host where the repo is checked out elsewhere. `README.md`,
`QUICKSTART.md`, and `MONITORING_GUIDE.md` all document this legacy
3-server/port-80 layout, not the current tiered/5-backend one described in
this spec.

## 7. Starting and deploying the system

Two independent ways to bring the infrastructure up exist in the repo;
they use different networks and are not interchangeable **(Observed
inconsistency, see §2)**:

1. **Docker Compose** (declarative, matches `docker-compose.yml`):
   ```bash
   docker-compose up -d
   ```
   Brings up `web_server_1`..`web_server_5` and `load_balancer` on the
   `lb_network` bridge, using the ports/IPs in §2.

2. **Control scripts** (imperative, matches what the dashboard drives):
   ```bash
   chmod +x control-scripts/*.sh
   ./control-scripts/start_servers.sh all   # backends 1-3 + any web_server_N>3
                                             # referenced in docker-compose.yml,
                                             # then the load balancer
   ```
   Individual pieces can be managed with `start_servers.sh`/
   `stop_servers.sh`/`restart_servers.sh <N>|lb|all`, and new backends can be
   added live with `add_server.sh` or `create_docker_server.sh`.

Either way, verify with `docker ps` and then reach the system via:
- `http://<host>:8082` — user tier, load-balanced site.
- `http://<host>:8080` — admin tier, dashboard with full control.
- `http://<host>:8081` — reader tier, read-only dashboard.

**Monitoring dashboard** (separate process, not containerized):
```bash
cd view
./run.sh
```
`run.sh` creates a Python venv on first run, installs
`requirements.txt` (Flask, docker, requests, psutil, Werkzeug), and runs
`python3 monitor.py`, which listens on `0.0.0.0:5000`. It can be reached
directly at `http://<host>:5000`, or indirectly through the load
balancer's admin/reader tiers (§4) once the load balancer container is
running — the load balancer proxies to it at `172.20.0.1:5000`, i.e. the
Docker host from the bridge network's point of view.

**Stopping**: `docker-compose down` (if started via Compose) or
`./control-scripts/stop_servers.sh all` (if started via the scripts); the
dashboard process is stopped with Ctrl+C or by killing `monitor.py`.
