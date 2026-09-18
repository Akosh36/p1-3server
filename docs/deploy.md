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

### Gateway/DHCP/NAT (Phase 3 — shipped)

Turns this host into the LAN's actual gateway to the internet, on top of
the Phase 2 access control (nothing about the ACL enforcement changes —
this only adds forwarding + address translation for devices that already
passed it).

**DHCP** — dnsmasq itself, configured, not wrapped:

```bash
sudo cp deploy/dnsmasq/p13server.conf.example /etc/dnsmasq.d/p13server.conf
sudo $EDITOR /etc/dnsmasq.d/p13server.conf   # set interface=, dhcp-range=,
                                              # dhcp-option=3/6 for your LAN
sudo systemctl enable --now dnsmasq
```

This is the same lease file path (`/var/lib/misc/dnsmasq.leases`) netdiscd
(Phase 1) already reads — no changes needed on that side. Verified with a
real dnsmasq instance in a throwaway namespace: a real `udhcpc` DISCOVER →
OFFER → REQUEST → ACK exchange produced a lease line in the exact format
`internal/netdisc`'s parser expects.

**NAT + gateway hardening** — set one more env var for fwctl and restart it:

```
Environment=FWCTL_WAN_INTERFACE=eth0    # your internet-facing interface
```

When `FWCTL_WAN_INTERFACE` is set, `internal/firewall.BuildRuleset` adds:

- `net.ipv4.ip_forward=1` (fwctl sets this itself on startup — without it
  the kernel never hands packets to the forward chain at all, regardless
  of nftables rules).
- A `nat_postrouting` chain (`type nat hook postrouting`) that masquerades
  everything leaving via the WAN interface — LAN devices reach the
  internet behind this host's single WAN address, exactly like a normal
  home router.
- `iifname != <wan>` prepended to **every** MAC-allow rule in both
  `lan_forward` and `management_input`. A MAC allow-list alone can't stop
  a spoofed source MAC arriving ON the WAN interface — this closes that
  gap so the "o'rtadagi server" (management ports) and LAN-only forwarding
  can never be reached from the internet side, even by an attacker who
  somehow knows an admin's real MAC.

**Verified with real traffic across a 3-namespace gateway topology**
(`lan` ↔ `gw` running the real dnsmasq + fwctl + NAT ↔ `wan` simulating the
internet): the DHCP-assigned LAN client was blocked until granted, then
reached the "internet" host once granted — with the internet host's own
access log confirming every request arrived from the **gateway's WAN
address**, never the LAN client's real IP. Revoking access blocked it
again. Separately, the WAN-hardening rule was tested by setting the WAN
namespace's own interface MAC to match a granted admin's MAC and
confirming management access was still refused — proving the `iifname`
guard, not just the MAC set, is what's stopping it.

### lbd (Phase 4 — shipped)

Real L4 TCP load balancing per server group, on top of everything above:
traffic that `lan_forward` (Phase 2) already allows through can now
actually reach a group's VIP and get proxied to a real backend, instead of
hitting a dead address.

```bash
go build -o /usr/local/bin/lbd ./cmd/lbd
sudo cp deploy/systemd/lbd.service /etc/systemd/system/
sudo $EDITOR /etc/systemd/system/lbd.service   # set LBD_INTERFACE to your LAN interface
sudo systemctl daemon-reload
sudo systemctl enable --now lbd
```

Like netdiscd and fwctl, lbd has no database access. Each VIP is a real
`/32` secondary address added directly to `LBD_INTERFACE` (`internal/lb/vip.go`,
`ip addr add <vip>/32 dev <iface>`) — a deliberate choice over a separate
dummy interface, so the kernel answers ARP for it exactly like any other
locally-owned address and LAN clients reach it like a normal host on the
subnet. lbd then binds a real `net.Listener` on `<vip>:<port>` and proxies
each accepted connection to a backend chosen by the group's algorithm
(`round_robin` or `least_conn`), health-checked every 3s with a plain TCP
connect probe (protocol-agnostic — works for HTTP, a game server, anything
that accepts TCP) and a 2-consecutive-failure/1-success flap-avoidance
threshold before a backend is pulled from or returned to rotation.

`cmd/api`'s `internal/lbsync.Run` is the only thing that ever talks to
Postgres for this: every ~3s it reads `server_groups`/`backend_servers`
(only `is_active = true` groups), pushes the desired VIP/backend list to
lbd's `/sync` endpoint over `/run/p13server/lbd.sock`, then pulls lbd's
`/status` endpoint and writes `is_healthy`/`last_check_at`/`response_time_ms`
back onto each `backend_servers` row — the same push/pull-over-Unix-socket
shape as `internal/aclsync` (fwctl) and `internal/discovery` (netdiscd).

**Verified with real traffic in a 4-namespace topology** (`lan` client ↔
`gw` running the real lbd ↔ `backend1`/`backend2`, each a real
`python3 -m http.server`): confirmed ARP resolves the VIP to the gateway
(`ip neigh show` → `REACHABLE`), then 6 sequential real `curl`s alternated
perfectly `BACKEND1-OK`/`BACKEND2-OK` under `round_robin`. Killing
backend1's process was picked up by the health check within two 3s ticks
(`GET /status` showed `is_healthy: false`), and live client traffic during
that window — not just the recorded health state — went to `BACKEND2-OK`
on all 6 requests; restarting backend1 rejoined it to the rotation on the
next successful probe. `least_conn` was verified by holding open several
raw TCP connections and confirming each new connection always went to
whichever backend currently had fewer active connections (`0-0 → 1-0 → 1-1
→ 2-1 → 2-2`, never lopsided). Removing a group (or setting
`is_active = false` in Postgres and waiting for the next `lbsync` tick) was
confirmed to release the VIP address and close the listener; the full
control-plane loop was also verified against a real local Postgres — real
`server_groups`/`backend_servers` rows, a real `cmd/api` process, its
`lbsync` push reaching a real lbd and its health pull landing back in the
same rows — including flipping `is_active` in the database and watching
the VIP start/stop accordingly with no direct Postgres access from lbd
itself.

That testing caught one real concurrency bug, now a load-bearing comment in
`internal/lb/manager.go`: a VIP listener and its health-check loop are
started inside `Manager.Sync`, which is called from the `/sync` HTTP
handler with `r.Context()`. Go's `net/http` cancels that context the
instant the HTTP response finishes writing — deriving the listener's
lifetime from it meant **every VIP closed itself immediately after each
successful sync**, which looked fine at the HTTP-response and `ip addr
show` level (200 OK, VIP address present, ARP resolving) but left no
listener at all behind it. Fixed by giving `Manager` a separate
daemon-lifetime `baseCtx` (set once at startup) that listener/health-check
goroutines derive from, while the request-scoped context is still used for
the synchronous `ip addr add/del` calls that need to finish before `/sync`
responds.

### backendagentd (Phase 5 — shipped)

Real CPU/RAM/disk/network metrics for each **backend** server sitting
behind a Phase 4 VIP — not the gateway itself (that's `internal/metrics`,
Phase 0's "Server" card). This is the one component in this repo that does
**not** run on this platform's own gateway host: it's a tiny agent binary
installed on each backend server, wherever that server actually lives, and
it reaches the control-plane API over the network instead of a local Unix
socket (there is no local socket to share — it isn't on the same machine).

```bash
# On the BACKEND server itself, not the gateway:
go build -o /usr/local/bin/backendagentd ./cmd/backendagentd
sudo cp deploy/systemd/backendagentd.service /etc/systemd/system/
sudo $EDITOR /etc/systemd/system/backendagentd.service   # set AGENT_API_URL
                                                           # and AGENT_TOKEN
sudo systemctl daemon-reload
sudo systemctl enable --now backendagentd
```

`AGENT_TOKEN` comes from the Servers page: adding a backend now generates a
random per-backend token automatically (shown once in the create response,
always visible afterward under that backend's "Metrikalar" panel, with a
"Tokenni yangilash" button to rotate it if it's lost or leaked — rotating
doesn't require deleting/recreating the backend, so its health history
survives).

Every `AGENT_INTERVAL` (default 10s) the agent samples its own host via
`internal/hostmetrics` (the same gopsutil-based sampler `internal/metrics`
uses for the gateway's own numbers — extracted into a shared package in
this phase so the logic is written once) and `POST`s the sample to
`/api/agent/metrics`, authenticated with `Authorization: Bearer
<agent_token>`. That endpoint is deliberately outside the JWT `requireAuth`
group in `internal/httpapi/server.go` — the caller is a machine on a
backend server, not a logged-in admin — and resolves the token straight
against `backend_servers.agent_token` before writing into the new
`backend_metrics` time-series table. A wrong or revoked token gets a plain
401; the agent logs one warning and keeps retrying rather than crashing
(same resilience pattern as `aclsync`/`discovery`/`lbsync` when their
daemon is unreachable).

**Verified end-to-end against a real local Postgres, not mocked:** a real
`cmd/api` process, a real `cmd/backendagentd` binary pointed at it with a
token minted through the real create-backend API call, pushing real
`gopsutil` samples every 2s that landed in `backend_metrics` and were
readable back through `GET /api/backends/{id}/metrics` — confirmed by
direct SQL query as well as the API response. Auth was verified three ways:
a wrong token and a missing `Authorization` header both got 401; hitting
`POST /backends/{id}/regenerate-token` immediately invalidated the old
token (401) while the new one started working (200) without restarting
`cmd/api`. The Servers page itself was checked in a real headless browser
(Playwright): the token displays and copies to the clipboard correctly,
"Tokenni yangilash" visibly rotates it, and the per-backend chart renders
real, live CPU/RAM/Disk lines once the agent is running — with an explicit
empty-state message (not a blank chart) for a backend that has no agent
pushing to it yet. Zero browser console errors throughout.

This same testing pass caught a **stale-data bug left over from Phase 4's
own manual testing**, not a Phase 5 code bug: a backend row's
`ip`/`port` had been temporarily repointed at a throwaway `ip netns`
target during Phase 4's `ip netns` validation and restored afterward, but
`is_healthy`/`last_check_at`/`response_time_ms` were never reset —
so the Servers page kept showing that backend as "Sog'lom" (healthy) with
a real-looking response time for an address nothing had actually checked
since. Phase 4's own UI fix (distinguishing "never checked" from "checked
and down") made this visible rather than hiding it, which is how it was
caught here; fixed by resetting those three columns to their true
never-checked state (`false`/`NULL`/`NULL`).

**Known limitation, not yet solved — flagged honestly rather than glossed
over:** `fwctl`'s `management_input` chain (Phase 2) restricts inbound
traffic to `FWCTL_MANAGEMENT_PORTS` on the gateway host to admin-MAC
devices only. If a backend server lives on the same LAN this gateway
firewalls (rather than on a separate server/management network, or reached
through a route that bypasses `lan_forward` entirely), its agent's pushes
to the control-plane API will be **silently dropped by that same firewall**
unless its MAC is admin-granted — which is not an access level a load-balanced
backend server should need just to report its own metrics. There is no
dedicated "metrics ingestion" allowance separate from the admin-MAC
management ports yet; for now, either place backend servers outside the
LAN segment `fwctl` controls, or grant the backend's MAC admin access as a
workaround, both with their own trade-offs. A proper fix (e.g. a narrower,
metrics-only nftables allowance) is future work, not part of this phase.

### capd (Phase 6 — shipped)

On-demand packet capture, per device, on top of everything above.

```bash
apt-get install -y tcpdump   # capd shells out to the real binary
go build -o /usr/local/bin/capd ./cmd/capd
sudo cp deploy/systemd/capd.service /etc/systemd/system/
sudo $EDITOR /etc/systemd/system/capd.service   # set CAPD_INTERFACE to your LAN interface
sudo systemctl daemon-reload
sudo systemctl enable --now capd
```

Like the other data-plane daemons, capd has no database access and is
deliberately "dumb": `internal/capdsync` (in `cmd/api`) owns every decision
— capture IDs, file names, when a segment rotates, what happens next — and
tells capd only "start this MAC into this file" / "stop that". capd's own
job is just running (or stopping) a real `tcpdump -i <iface> -U -w <path>
ether host <mac>` process and reporting when a running capture has crossed
its configured size/time threshold; it never rotates or restarts anything
on its own. Filtering by MAC (not IP) survives a DHCP lease renewal, and
capturing on the LAN-facing interface sees both directions of a device's
traffic, since this platform is that device's default gateway (decision
#14) — every packet either arrives from the device addressed to the
gateway's own MAC, or leaves the gateway addressed to the device's MAC.

`internal/capdsync`'s reconciliation loop (every ~2s) is the same shape as
`lbsync`/`aclsync`: it compares `traffic_captures` rows with
`status = 'recording'` against capd's live status, (re)starts any that
aren't actually running yet (self-healing if capd was briefly down), and
rotates any that have crossed their threshold — ending the current segment
(`status = 'rotated'`, with a reason) and immediately starting a fresh one
for the same device, so recording never visibly stops except via an
explicit "Stop". The Logs page's "Download" button drives the exact same
rotation function with `status = 'downloaded'` instead, which is how the
spec's "Download pauses the recording, finalizes the file, then a new file
starts" behavior is implemented — one function, two callers, one reason
string different. `POST /api/captures/{id}/stop` is the one case that
does *not* start a continuation: it flips the row to `'stopped'` *before*
telling capd to stop, specifically so a concurrent reconciliation tick
(which only ever touches rows still marked `'recording'`) can never race
to "helpfully" restart it.

Disk usage is capped two ways, both from decision #8: `CAPD_ROTATE_MB` /
`CAPD_ROTATE_SECONDS` bound any single segment's size and age, and
`CAPD_MAX_TOTAL_MB` bounds the total across every device's captures —
enforced by capd itself scanning `CAPTURE_DIR` every 10s and deleting the
oldest completed files (by mtime) until back under the cap, never a file
still being written to. Evicted paths are reported back to
`internal/capdsync`, which marks the matching row `status = 'error'` with
`rotation_reason` noting `quota_evicted` (appended to whatever reason was
already there, e.g. `"size_limit; quota_evicted"`) — an honest record that
the file is gone, not a silently broken download link.

**Verified with a real `tcpdump` process against a real veth pair, then
against a real Postgres + real `cmd/api` + a real browser.** In an
`ip netns` `lan`↔`gw` topology, a capture on `lan`'s real MAC correctly
recorded a real `ping` exchange in both directions — read back with
`tcpdump -r`, exactly as Wireshark would open it. Transferring a real 5MB
file triggered `needs_rotation: true` at a 1MB threshold, and separately
confirmed a real 3s time-based rotation. The full control-plane loop was
then exercised through the actual HTTP API against a real local Postgres:
`POST /api/devices/{id}/captures` started a real capture picked up by the
next `capdsync` tick; `GET /api/captures/{id}/download` on a live
recording correctly finalized it (`status = 'downloaded'`), started a
continuation, and served back a valid, readable pcap; `POST
/api/captures/{id}/stop` ended a capture with no continuation and no
resurrection race; and a 1MB `CAPD_MAX_TOTAL_MB` correctly evicted the
oldest files (skipping the one still being written) while marking their
rows `status = 'error'`. The Users and Logs pages were checked in a real
headless browser (Playwright): starting a capture flips the row to a live
ticking duration with a Stop button, clicking "Yuklab olish" on the Logs
page downloads a real file through the browser (verified afterward with
`tcpdump -r`) while the table updates to show the old segment as
downloaded and a new one recording — zero console errors throughout.

**Real concurrency bug found by this testing, not by unit tests:**
`Manager.Stop` spawned its own goroutine calling `cmd.Wait()` on the
tcpdump process, while the `watch` goroutine started back in `Start` was
*also* calling `cmd.Wait()` on that same process — something Go's
`os/exec` docs explicitly forbid ("incorrect to call Wait concurrently
with any other method on the Cmd"). Under light traffic (a few pings) one
call usually reaped the process first and the other harmlessly lost the
race, so this didn't surface immediately. Under heavier traffic (tcpdump
taking longer to flush and exit after the 5MB transfer), the two calls
could each end up waiting on the other's result, hanging
`POST /captures/{id}/stop` forever. Fixed by making `watch` the *only*
caller of `cmd.Wait()`, closing a `done` channel when it returns; `Stop`
now just waits on that channel instead of calling `Wait()` a second time.

**Known limitations:** rotation/quota thresholds are configured in whole
megabytes (`CAPD_ROTATE_MB`, `CAPD_MAX_TOTAL_MB`) even though the
underlying tracking is byte-precise — a config-level rounding choice, not
a code limitation. Capture of a device connected via the future Phase 7
WireGuard tunnel hasn't been tested (no such tunnel exists yet).

### Still not shipped: WireGuard site-to-site (Phase 7)

Adds:

1. WireGuard configuration (`wg-quick` or an equivalent) for the tunnel
   itself — likely orchestrated rather than a new Go daemon, following
   decision #4's "build on ready-made tools" precedent (dnsmasq for DHCP).
2. Wiring `lan_networks`/`vpn_peers` (already in the schema since Phase 0)
   to real Active/Deactive control and a real-vaqt reachability check.

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
