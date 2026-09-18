# Deployment

Two halves, split by privilege — never merge them:

- **Control-plane** (no special OS privileges): PostgreSQL, the REST API
  (`cmd/api`), the React admin panel. Runs in Docker via
  `deploy/docker/docker-compose.yml`.
- **Data-plane** (root / `CAP_NET_ADMIN`, needs host networking): `netdiscd`,
  `fwctl`, `lbd`, `capd`, `wgd`. Runs as systemd units directly on the
  Ubuntu gateway host — never in a container — and exposes a
  localhost-only Unix control socket under `/run/p13server/` that the API
  reads/writes.

This split exists so the API's own attack surface never carries the
privileges its network operations need; see `CLAUDE.md` (sections 4 and
11) for the full architecture rationale.

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

### wgd (Phase 7 — shipped)

Real site-to-site WireGuard: a second, physically remote LAN bridged to
this one over an encrypted tunnel, with the LAN page's Active/Deactive
button and reachability indicator now backed by an actual daemon instead
of a bare database flag.

```bash
apt-get install -y wireguard-tools   # wgd shells out to the real `wg` CLI
# wireguard-go only if this kernel lacks native WireGuard support (rare on
# a stock Ubuntu Server LTS — mainline since Linux 5.6):
apt-get install -y wireguard-go
go build -o /usr/local/bin/wgd ./cmd/wgd
sudo cp deploy/systemd/wgd.service /etc/systemd/system/
sudo $EDITOR /etc/systemd/system/wgd.service   # review WGD_ADDRESS/WGD_LISTEN_PORT
sudo systemctl daemon-reload
sudo systemctl enable --now wgd
```

Like the other data-plane daemons, wgd has no database access and follows
decision #4 (build on ready-made tools): it doesn't reimplement WireGuard's
crypto or protocol, it drives the real `wg`/`ip` CLIs (and generates its
own long-lived private key with `wg genkey` the first time it starts,
persisting it so this gateway's identity survives restarts — remote peers
know it by the derived public key). `internal/wgsync` (in `cmd/api`) is
the only thing that reads Postgres: every ~3s it computes which
`vpn_peers` should currently be active — the peer itself must be active
**and**, if a `lan_networks` row points at it (the LAN page's own
Active/Deactive toggle), that row must be active too — pushes that list to
wgd's `/sync`, and writes real handshake-based reachability back into
`vpn_peers.last_handshake_at` / `lan_networks.is_reachable`.

A peer counts as "reachable" if it handshook within `WGD_REACHABLE_AFTER_SECONDS`
(default 150s); every peer gets `PersistentKeepalive=25s` so this stays
close to real-time even with no LAN traffic actually flowing, satisfying
the spec's "real-vaqtda, kam kechikish bilan" requirement without needing
to ping into the remote subnet at all. Creating a remote LAN from the LAN
page (`POST /api/lan-networks/remote-vpn`) creates both the `vpn_peers`
row (the remote site's public key, its LAN subnet as `AllowedIPs`, and its
endpoint) and the `lan_networks` row together in one step — this app never
reuses one peer across multiple LAN entries, so there was no reason to
make that two separate admin actions. `GET /api/wireguard/local-info`
exposes wgd's own public key/port so the admin can hand it to the remote
site's admin (site-to-site WireGuard needs both ends configured with each
other's public key).

**One thing `wg` (unlike `wg-quick`) does not do: manage routes.** Setting
a peer's `AllowedIPs` only tells WireGuard's own cryptorouting which
packets belong to which peer — it does not touch the kernel routing
table. `internal/wg`'s `Sync` explicitly runs `ip route replace <remote
subnet> dev wg0` for every active peer (and removes it when the peer is
removed) to cover the gap that `wg-quick`'s wrapper script normally
papers over. No changes were needed in `internal/firewall`: `lan_forward`'s
existing `ct state established,related accept` plus its MAC-source-match
rules (added in Phase 2, unmodified since) don't restrict by egress
interface at all, so traffic from an already-granted LAN device toward a
peer's subnet via wg0 is accepted by the exact same rules that already
cover WAN and backend-VIP traffic — verified by reading the generated
ruleset rather than by re-running the Phase 2 test suite a second time.

**Verified with a real, fully encrypted tunnel between two simulated
gateways, then with the real control-plane loop and a real browser.** A
4-namespace topology (`siteA` ↔ `gwA` ↔ [simulated internet] ↔ `gwB` ↔
`siteB`) was built with real IP addresses on both "public" and "LAN"
sides. Each `gwd` instance generated its own real keypair; configuring
them as each other's peer produced a real WireGuard handshake, and `ping`
from `siteA` to `siteB` succeeded end-to-end (TTL 62, confirming two real
routed hops) — with `tcpdump` on the simulated internet link showing
**only opaque UDP/51820 packets**, no plaintext ICMP at all, confirming
genuine encryption rather than an accidental unencrypted passthrough.
Removing the peer immediately cut connectivity and removed the route;
re-adding it restored both. The full control-plane loop was then exercised
against a real local Postgres: `POST /api/lan-networks/remote-vpn`
created both rows, the next `wgsync` tick pushed the peer and the tunnel
came up — again with real cross-site `ping` succeeding — and
`is_reachable`/`last_handshake_at` were confirmed correct in the database.
Toggling `is_active` via the same `PATCH` endpoint the LAN page's button
calls tore the tunnel down and flipped `is_reachable` to `false` honestly
(not just "stopped updating"); re-toggling restored it; `DELETE` removed
both rows and the peer. The LAN page itself was checked in a real headless
browser (Playwright): the local public key displays and copies, the
"+ Masofaviy LAN qo'shish" form creates a real tunnel, the expandable
detail row shows the peer's real config and handshake time, and the
Active/Deactive toggle and delete both work — zero console errors
throughout.

**Real, non-code discovery from this testing:** the userspace `wireguard-go`
fallback's control socket lives at a fixed, host-wide path
(`/run/wireguard/<ifname>.sock`) that is **not** network-namespace-scoped.
Running two `wgd` instances in separate namespaces on the same host with
the same interface name (`wg0`) collided on that path and the second
instance's interface never appeared. This only matters for exactly this
kind of same-host multi-namespace testing — in a real deployment each
site-to-site gateway is a separate machine, so the path is never shared —
but it's why the test topology above uses distinct interface names
(`wgA0`/`wgB0`) per side, and why deploying two wgd instances on one real
host (unusual, but possible) would need distinct `WGD_INTERFACE` values
too if either ever falls back to userspace mode.

**Known limitations:** `AllowedIPs`/routing here only carries one CIDR per
peer (`vpn_peers.allowed_subnet` is a single `CIDR` column) — a remote
site with multiple non-contiguous subnets needs one `vpn_peers`/`lan_networks`
pair per subnet, which works but isn't the most convenient shape.
Capturing a device's traffic (Phase 6, capd) when that device sits behind
a WireGuard tunnel hasn't been tested, since capd filters by Ethernet MAC
and a device on the far side of a routed L3 tunnel never puts its own MAC
on this gateway's LAN interface — matching capd's documented scope (LAN
devices only, not remote-VPN ones).

### Per-user backend traffic accounting + GPU metrics (Phase 8 — shipped)

No new daemon or systemd unit — this phase adds one migration
(`0004_gpu_metrics.up.sql`) and extends two existing components already
covered above: `lbd`/`lbsync` (Phase 4) and `internal/hostmetrics`
(shared by Phase 0's `internal/metrics` and Phase 5's `backendagentd`).
Run the migration by restarting `cmd/api` (migrations apply automatically
on startup) — no config changes needed for either existing daemon.

**Traffic accounting:** `lbd` now counts real bytes in both directions for
every proxied TCP connection (`io.Copy`'s return value, via `atomic.Int64`)
and accumulates them in memory keyed by `(vip_address, client_ip)`, exposed
over a new `GET /traffic` endpoint on the same Unix socket as `/sync` and
`/status`. Reading it **drains** the counters (each poll returns everything
since the last one), so `lbd` never needs to track "already reported"
state itself. `internal/lbsync` polls this every tick alongside the
existing push/pull, and resolves each `(vip_address, client_ip)` pair
against `server_groups`/`devices` with a single `INSERT ... SELECT`
(`device_traffic_stats`, a table that has existed since Phase 0's initial
schema but was never written to until now) — an entry whose IP doesn't
match a known device, or whose VIP doesn't match an active group, is
silently dropped rather than guessed at. A new endpoint,
`GET /api/devices/{id}/traffic`, aggregates that table per server group
(`SUM(bytes_in)`, `SUM(bytes_out)`, `MAX(time)`) for the Users page's new
"Trafik" expandable row.

**Verified with real traffic in a 2-namespace topology** (`lbtest-cli` ↔
`lbtest-gw` running the real `lbd` + a real `python3 -m http.server`
backend): a real 200 KB file was fetched through the VIP from the client
namespace with matching `md5sum` on both ends (the proxy didn't corrupt
anything); the next `lbsync` tick wrote a `device_traffic_stats` row with
`bytes_in=200205`/`bytes_out=89` (file + HTTP headers vs. a small GET) for
the correct `device_id`/`backend_group_id`; a second fetch produced a
second, independent row that `GET /api/devices/{id}/traffic` correctly
summed (`bytes_in=400410`/`bytes_out=178`); and a fetch from a second
client IP with no matching `devices` row produced no new row and no error
in `cmd/api`'s log, confirming the "skip unmatched" behavior end-to-end.

**GPU metrics:** `internal/hostmetrics` gained a `gpuSampler` that shells
out to `nvidia-smi` once available (cached after the first
`exec.LookPath` check) and averages `utilization.gpu`/`utilization.memory`
across however many GPUs are present. When `nvidia-smi` isn't on `PATH` or
the command fails, both fields come back as a Go `nil`, which the
`gpu_percent`/`gpu_mem_percent` columns (nullable on both `system_metrics`
and `backend_metrics`) and the JSON API (`omitempty`) all preserve as a
real "no GPU," never a fabricated `0`. Both `internal/metrics` (the
gateway's own "Server" card) and `cmd/backendagentd` (unchanged — it
already forwards the whole `hostmetrics.Sample` struct) pick this up
automatically.

**Verified for real, honestly, given no GPU hardware exists in this
environment** (confirmed absent: no `nvidia-smi`, no `/dev/nvidia*`, no
`lspci` output for a GPU): a live `cmd/api` + Postgres round-trip through
`GET /api/metrics/self` confirmed `gpu_percent`/`gpu_mem_percent` are
absent from the JSON response entirely, matching the "not a fake zero"
design. The CSV-parsing path itself (which can't be exercised by real
hardware here) was tested against a **real subprocess** — a fake
`nvidia-smi` shell script placed on `PATH` returning two GPUs' worth of
canned utilization figures — rather than mocking `exec.Command` away, so
the actual flag/format assumptions are what's under test
(`internal/hostmetrics/gpu_test.go`). The genuinely-GPU-present path still
needs validation against real NVIDIA hardware before relying on it in
production.

### LAN network auto-discovery + drill-down (Phase 9 — shipped)

No new daemon — this phase extends `internal/discovery` (the control-plane
half of Phase 1's `netdiscd`) and the LAN page. Migration
`0005_lan_network_unique.up.sql` adds a `UNIQUE (type, name)` constraint to
`lan_networks` so the new upsert logic can use the same `ON CONFLICT`
pattern `upsertSwitchPorts` already uses, rather than a racy
select-then-insert.

Every reconciliation tick, `internal/discovery.reconcile` now also ensures
one `lan_networks` row (`type='wired'`) per distinct switch reported in
the snapshot, and one row (`type='wireless_local'`) per distinct SSID —
then resolves each device's `lan_network_id` to the matching row (or, for
a wired device netdiscd couldn't tie to a specific switch — a real,
already-documented limitation: many switches' SNMP agents don't expose
`dot1dTpFdbTable`, the MAC-to-port table — a single catch-all "port
aniqlanmagan" network, so no wired device is ever left without a LAN
network at all). A closing step recomputes `is_reachable` for every
wired/wireless_local network as "does it currently have at least one
online device" — the same real online/offline signal Phase 1 already
tracks per-device, just aggregated; remote VPN networks' `is_reachable`
is untouched here (that's still `wgsync`, Phase 7). None of this ever
deletes a row: a switch or SSID that stops appearing just settles to
`is_reachable = false`, matching how a device itself goes offline instead
of disappearing.

On the LAN page, every switch-port row and every LAN-network row got a
"Qurilmalar" (devices) button — clicking it filters the devices table
below to just that port's or network's devices, with a "Filtrni tozalash"
button to go back to the unfiltered list. Auto-discovered rows
(`wired`/`wireless_local`) no longer show the Active/Deactive toggle or a
delete button — there's no daemon that could act on either for a physical
switch or WiFi AP, and deleting one would just have `discovery`'s next
tick recreate it; those controls remain only on `wireless_remote_vpn`
rows, which really do have `wgd` behind them.

**Verified with a real Postgres and a real `cmd/api`, against a
purpose-built fake `netdiscd`:** netdiscd's own collectors (ARP, SNMP,
hostapd) were already verified against real tooling in Phase 1 — what's
new here is purely `reconcile`'s handling of the snapshot it receives, so
a small standalone Go program was written to serve the exact same
`GET /snapshot` JSON contract over a real Unix socket at
`/run/p13server/netdiscd.sock`, standing in for netdiscd the way a plain
`python3 -m http.server` has stood in for a "backend" in earlier phases'
tests. Fed a snapshot with two ports on one switch, one wired device tied
to a port, one wired device with no switch info at all, two
`wireless_local` devices sharing one SSID (one online, one not), and one
on a second SSID (offline): the resulting `lan_networks` rows matched
exactly — the named switch and the "port aniqlanmagan" catch-all both
`is_reachable = true` (each had an online device), the shared SSID
`is_reachable = true`, the lone offline SSID `is_reachable = false` — and
every device's `lan_network_id`/`switch_port_id` was correct both by
direct SQL and through `GET /api/devices`. Running two reconciliation
ticks back-to-back produced no duplicate rows. The LAN page was checked in
a real headless browser (Playwright): clicking a specific port's
"Qurilmalar" button showed only the device on that port (not the one with
no port info); clicking a specific SSID's showed only that SSID's two
devices (not the other SSID's); auto-discovered rows correctly showed
"— (avtomatik)" and "(o'chirib bo'lmaydi)" instead of live controls.

**Known limitations:** a `wireless_remote_vpn` network's "Qurilmalar"
button always shows an empty list — as documented in Phase 7, devices
behind a WireGuard tunnel are never tracked as `devices` rows at all, so
there's nothing for this drill-down to find; that's this feature's real
scope (netdiscd-observed LAN devices), not a bug. Wired grouping is
per-switch only, not per-VLAN, even though `switch_ports.vlan` exists —
this sandbox has no VLAN-aware switch to validate finer-grained grouping
against, and the spec doesn't call for it.

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
