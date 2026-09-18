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

### netdiscd (Phase 1 — shipped)

```bash
go build -o /usr/local/bin/netdiscd ./cmd/netdiscd
sudo cp deploy/systemd/netdiscd.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now netdiscd
```

netdiscd has no database access at all (see CLAUDE.md §4) — it only
publishes a snapshot on `/run/p13server/netdiscd.sock`, which `cmd/api`'s
`internal/discovery.Run` polls every 5s and upserts into `devices` and
`switch_ports`. It merges four independent, all-optional sources:

| Source | What it needs | What it gives |
|---|---|---|
| ARP table (`/proc/net/arp`) | Nothing (always available on Linux) | IP↔MAC, "is it here right now" for wired devices |
| dnsmasq lease file | `NETDISC_DNSMASQ_LEASE_FILE` (default `/var/lib/misc/dnsmasq.leases`) | Hostname, a fallback IP |
| SNMP (managed switch) | `NETDISC_SNMP_TARGET` set to `ip:161` | Per-port link status (IF-MIB) and, if the switch exposes it, MAC→port (BRIDGE-MIB `dot1dTpFdbTable`) |
| hostapd control socket | `NETDISC_HOSTAPD_SOCKET_DIR` set (e.g. `/var/run/hostapd`) | Which MACs are currently associated to the local AP |

Any source left unconfigured is simply skipped — netdiscd runs fine with
just the ARP table on a LAN with no managed switch or local AP yet. A
device is never deleted once seen; going quiet only flips `is_online` to
false after `NETDISC_STALE_AFTER` (default 90s), so an admin can still find
and revoke a device that has stepped away.

Granting a device's User/Admin role happens in the admin panel's **LAN**
page ("Barcha aniqlangan qurilmalar" table) — that PATCH is what `fwctl`
(Phase 2) will read to build its nftables allow-lists.

### fwctl (Phase 2 — shipped)

```bash
go build -o /usr/local/bin/fwctl ./cmd/fwctl
sudo cp deploy/systemd/fwctl.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now fwctl
```

fwctl also has no database access — it applies a **deny-all baseline the
instant it starts** (so a fresh boot or a crash never means "no rules
loaded = everyone gets through"), then waits for `cmd/api`'s
`internal/aclsync.Run` to push the real allow-lists over
`/run/p13server/fwctl.sock` every ~2s, reading straight from
`devices`/`access_grants` (the single source of truth — CLAUDE.md §11).

It owns two nftables chains in its own `inet p13server` table, so
`FWCTL_TABLE_NAME` namespaces everything it touches away from the rest of
the host's ruleset:

| Chain | Hook | Enforces |
|---|---|---|
| `lan_forward` | `forward` | Only MACs in `allowed_user_mac` or `allowed_admin_mac` may have traffic forwarded through this gateway at all — everyone else hits the chain's `policy drop`. |
| `management_input` | `input` | Only `allowed_admin_mac` may reach `FWCTL_MANAGEMENT_PORTS` (default `22,8080`) on this host itself — the "o'rtadagi server". A `user`-role MAC passes `lan_forward` but is dropped here, exactly matching the access matrix in CLAUDE.md §4. |

Both chains also carry the DDoS baseline from decision #16 (smart
defaults, not user-configurable — the exact numbers and reasoning are
comments in `internal/firewall/ruleset.go`): an ICMP echo-request rate
limit, and a per-MAC (or per-source-IP for the management port) new
connection-rate meter, so one misbehaving device can't starve the rest of
the LAN or take down the gateway's own management access.

**Verified against real nftables, not just unit tests.** Every rule above
was validated in a throwaway `ip netns` topology (`lan` device ↔ `gw`
running the real ruleset ↔ `backend` target) with real curl traffic across
the full grant → forward-chain-allowed → input-chain-still-blocked →
admin-upgrade → revoke → re-grant cycle, and again through the real
`internal/aclsync` → Postgres path. That testing caught two real nftables
semantics that aren't obvious from the docs and are now load-bearing
comments in `internal/firewall/ruleset.go`:

1. An inline `meter name { ... }` in a rule creates a named set the first
   time it runs; `flush table` clears rules but not that set, so the next
   reconciliation's implicit re-creation collides ("File exists"). Fixed
   by pre-declaring each meter with `add set ... { flags dynamic; }` and
   referencing it with `update @name { ... }` instead.
2. `flush table` does **not** clear an existing named set's *elements* —
   only chains/rules. Without an explicit `flush set` per set before
   `add element`, a revoked device's MAC would never actually leave
   `allowed_user_mac`/`allowed_admin_mac`.

### Still not shipped: lbd (Phase 4), capd (Phase 6)

Each adds:

1. The Go binary under `cmd/<name>`.
2. A systemd unit in `deploy/systemd/<name>.service` (runs as `root`, or the
   minimum capability set the daemon actually needs — documented in that
   unit file).
3. Whatever OS package it orchestrates (`wireguard-tools`).

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
