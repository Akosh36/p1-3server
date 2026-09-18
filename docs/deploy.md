# Deployment

Two halves, split by privilege — never merge them:

- **Control-plane** (no special OS privileges): PostgreSQL, the REST API
  (`cmd/api`), the React admin panel. Runs in Docker via
  `deploy/docker/docker-compose.yml`.
- **Data-plane** (root / `CAP_NET_ADMIN`, needs host networking): `netdiscd`,
  `fwctl`, `lbd`, `capd`. Runs as systemd units directly on the Ubuntu
  gateway host — never in a container — and exposes a localhost-only Unix
  control socket under `/run/p13server/` that the API reads/writes.

This split exists so the API's own attack surface never carries the
privileges its network operations need; see `docs/architecture.md`
(added as each daemon ships) for why.

## Control-plane (available now, Phase 0)

```bash
cd deploy/docker
cp .env.example .env   # fill in POSTGRES_PASSWORD, JWT_SECRET, and the
                        # bootstrap admin credentials for the first run
docker compose up -d --build
```

The API applies its own migrations on startup and creates the first
`super_admin` from `BOOTSTRAP_ADMIN_USERNAME`/`BOOTSTRAP_ADMIN_PASSWORD` if
the `admins` table is empty. Unset those two variables after the first
successful login.

The panel is served on `:8443` by the `web` container (put a real
TLS-terminating reverse proxy — e.g. Caddy or nginx with a certificate — in
front of it before exposing this to the LAN; `web.Dockerfile` ships a plain
HTTP nginx for local/dev use only).

## Data-plane (systemd units — added per phase)

Not shipped yet: `netdiscd` (Phase 1), `fwctl` (Phase 2), `lbd` (Phase 4),
`capd` (Phase 6). Each phase adds:

1. The Go binary under `cmd/<name>`.
2. A systemd unit in `deploy/systemd/<name>.service` (runs as `root`, or the
   minimum capability set the daemon actually needs — documented in that
   unit file).
3. Whatever OS package it orchestrates (`dnsmasq`, `nftables`, `wireguard-tools`).

Until then, the "Server" page's live metrics, and every CRUD page (devices,
admins, server groups, LAN networks), work end-to-end against the
control-plane alone — they just reflect whatever is in the database rather
than live network state.

## Local development (no Docker)

```bash
# Postgres: any local instance works, e.g.
sudo -u postgres createuser p13server -P
sudo -u postgres createdb p13server -O p13server

export DATABASE_URL='postgres://p13server:<password>@127.0.0.1:5432/p13server?sslmode=disable'
export JWT_SECRET=$(openssl rand -hex 32)
export BOOTSTRAP_ADMIN_USERNAME=superadmin
export BOOTSTRAP_ADMIN_PASSWORD='<pick one>'
go run ./cmd/api

# in another terminal
cd web && npm install && npm run dev
```

The Vite dev server proxies `/api/*` to `http://127.0.0.1:8080` (see
`web/vite.config.ts`), so the frontend and API can be developed against each
other without CORS friction.
