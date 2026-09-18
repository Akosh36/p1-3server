# CLAUDE.md — LAN Access & Load-Balancing Control Platform

> Bu fayl loyihaning **to'liq konteksti**: asl g'oya, barcha arxitektura qarorlari
> (sabablari bilan), hozirgi holat va git tarixi. Har qanday yangi Claude Code
> sessiyasi (yoki inson) shu faylni o'qib, butun loyihani noldan tushunib olishi
> kerak — chunki bu loyiha bir nechta uzun suhbat davomida, ko'plab aniqlashtiruvchi
> savol-javoblar orqali shakllangan, va oraliq qarorlarning ko'pi kodning o'zida
> yozilmagan.

---

## 1. Loyiha nima va nima uchun kerak

Foydalanuvchi (loyiha egasi) **Zabbix'ga o'xshash, lekin ancha soddaroq va
tushunarli** tarmoq boshqaruv tizimi so'radi: bitta LAN tarmog'ida ishlaydigan,
**Active Directory'ga o'xshash — lekin 3 emas 2 xil** foydalanuvchi turiga
(oddiy **User** va **Admin**) ega, load balancing qiladigan, DHCP/firewall/VPN
bilan LAN'ni to'liq nazorat qiluvchi va admin uchun chiroyli real-vaqt
monitoring paneli beruvchi platforma.

Loyiha ikki marta katta burilish qildi:
1. Boshida repo oddiy 3-server nginx load-balancer demo edi (Docker + shell
   skriptlar + Flask monitoring dashboard).
2. Foydalanuvchi **butunlay yangi, professional darajadagi** tizim so'radi va
   **eski demo'ni to'liq o'chirib, noldan qurishga ruxsat berdi** ("loyihani
   ideyasi qoldirib strukturasini to'liq o'zgartiramiz... hatto to'liq o'chirib
   tashlashingga ham ruxsat beraman").

---

## 2. Asl talab (foydalanuvchining o'z so'zlari bilan, tuzilgan holda)

### 2.1. Umumiy g'oya
- Zabbix'ga o'xshash, lekin **sodda va tushunarli interfeys**, chiroyli grafiklar.
- Bitta tarmoqda **2 xil foydalanuvchi huquqi**: oddiy **User** va **Admin**
  (AD'dagi kabi ko'p darajali emas — faqat shu ikkitasi).
- Bu huquqlar **2 ta LAN tarmog'iga ulangan qurilmalarga** beriladi (mahalliy
  simli/wireless LAN + masofaviy wireless LAN, VPN orqali).
- Adminlar uchun **alohida admin paneli**, userlar uchun **alohida user
  paneli** — lekin bu domain/`ip/index.html`da bo'lishi, panel manzili
  shunchaki yashiringan (obscure) bo'lishi mumkin.
- **Faqat adminlar** userlarni **IP yoki MAC manzili bo'yicha** qo'sha oladi.
- LAN ulanganda tizim barcha qurilmalarga **dinamik IP tarqatishi** (DHCP)
  va adminga barcha IP, shu IP'dagi MAC manzil va hostname (qurilma nomi)ni
  ko'rsatib turishi kerak.
- **User yoki admin guruhida bo'lmagan** IP/MAC — orqadagi serverlarga
  **umuman kira olmaydi** (default-deny).
- **Userlar uchun "o'rtadagi server" umuman ishlamaydi** (kira olmaydi).
- **Adminlar uchun "o'rtadagi server" ishlaydi** (kira oladi).
- Userlarga **load balancing** qilinib beriladi (faqat LB serverlarga).
- Adminlar ham LB serverlarga kira oladi, **shu bilan bir qatorda o'rtadagi
  serverga ham** kira oladi.
- Boshqa hech kim (na user, na admin) hech qaysi tarafga kira olmaydi.

> **"O'rtadagi server" nima?** — Aniqlashtirilgan javob: bu boshqaruv/monitoring
> serverining **o'zi** (ya'ni shu platforma joylashgan asosiy server). Adminlar
> unga to'g'ridan-to'g'ri kira oladi (masalan boshqaruv porti/paneli orqali),
> userlar esa faqat load balancer orqali o'tadi va bu serverning o'ziga
> hech qachon bevosita chiqolmaydi.

### 2.2. Strukturani to'liq o'zgartirish talabi
- **Docker kerak emas** — asosiy e'tibor o'rtadagi load balancing'ga.
- Asosiy xizmatlar (mustaqil komponentlar sifatida):
  1. **Load balancing xizmati** — o'ziga ulangan barcha web yoki istalgan
     turdagi serverni userlar uchun barqaror ishlatib beradi.
  2. **Firewall** — LAN'da bo'lib turib, user yoki admin guruhiga
     qo'shilmagan barcha IP/MAC'ni bloklaydi.
  3. **Tarmoq aniqlash (netdiscover-ga o'xshash) xizmati** — ulangan LAN'dagi
     barcha qurilmalarni tinimsiz tahlil qilib, admin paneliga minimal
     uzilish bilan (real-vaqtga yaqin) ko'rsatib turadi.
  4. **Veb interfeys** — **faqat adminlar uchun**, userlarga butunlay yopiq.
     Juda tushunarli va sodda bo'lishi kerak, faqat kerakli qismlar. Bosh
     sahifada kategoriyalarga ajratilgan **kvadrat bo'limlar**: **Server,
     Userlar, Adminlar, Serverlar, LAN, Firewall, Logs**.

### 2.3. Har bir bo'lim uchun talablar

| Bo'lim | Talab |
|---|---|
| **Server** | Load balancing serverining o'zi haqida to'liq ma'lumot: CPU, GPU, internet tezligi, hard disk o'qish/yozish holati — hammasi **chiroyli grafik metrikalar** ko'rinishida. |
| **Userlar** | Jadval: **Nickname** (admin beradi) \| **Username/device name** \| **IP** \| **MAC**. Userga bosilganda: serverga qancha yuklama keltirayotgani, qaysi serverga kirib qancha trafik ishlatayotgani. **Eng muhim qism:** tugma bosilganda shu userning trafigini **pcap/Wireshark formatida** yozib oladi. Fayl admin panelining **Logs** bo'limiga tushadi. |
| **Adminlar** | Userlar bilan bir xil, faqat **trafik yozib olish qismi kerak emas**. |
| **Serverlar** | Asosiy load-balancing target serverlar. Bir nechta serverni asosiy LB serverga bo'lib qo'yish mumkin — IP+port orqali, **TCP protokoli** bilan, web/DB/istalgan resurs uzatiladi. Qo'shimcha: bir nechta bir xil serverni **bitta guruhga** birlashtirish — userlar uchun **bitta IP**, orqasida load balancing bilan taqsimlanadigan N ta server. Guruhlarga **nickname va rang** berish imkoniyati. |
| **LAN** | 2 ta jadval: **Portlar** (kabelli, boshqariladigan switch) va **Wireless**. Har bir port/IP(LAN)ga bosilganda o'sha LAN ichidagi barcha qurilmalar ko'rinadi. 2 xil o'zgartirish: 1) **nom tahrirlash**, 2) **huquq berish** (User/Admin — ikkalasi ham berilmasa, qurilma hech qayerga kirolmaydi). **Wireless (masofaviy) LAN**: bitta server, orqasida uzoqdagi LAN, simli emas — ikkita serverni **internet orqali VPN** bilan bog'laymiz (xavfsizlik uchun shifrlangan). Har bir LAN uchun **Active/Deactive** tugmasi + juda oddiy status, **real-vaqtda, kam kechikish bilan** mavjudligini tekshirish. |
| **Firewall** | Faqat user va adminni o'tkazadi, **DoS/DDoS** hujumlarga bardoshli. |
| **Logs** | Har bir user uchun trafik yozuvi: ism/familiya (nickname) + fayl. Pastda: qancha vaqtdan beri yozilayotgani (**Download** bosilmasa vaqt davom etaveradi). **Download** bosilganda: yozib olish vaqtincha to'xtaydi, fayl to'liq holatga keltirilib yuklanadi, so'ng **yangi faylga yozish davom etadi**. |

- Docker faqat kerak bo'lsa ishlatiladi (majburiy emas).

---

## 3. Qaror jurnali — barcha aniqlashtiruvchi savol-javoblar

Loyihani "kuchaytirish uchun maksimal savol ber" so'ralgach, 3 bosqichda 16 ta
savol berildi. Javoblar quyidagicha va **barcha keyingi dizayn shularga
asoslangan**:

| # | Savol | Qaror |
|---|-------|-------|
| 1 | Muhit | **Real production server** (jismoniy/virtual, ko'p NIC) |
| 2 | "O'rtadagi server" nima | **Boshqaruv/monitoring serverining o'zi** |
| 3 | Tech stack | Ochiq qoldirildi → Claude tanladi (professional darajada) |
| 4 | DHCP/Firewall | **Tayyor vositalar** (dnsmasq + nftables) ustida qurish |
| 5 | Tarmoq uskunasi | **Boshqariladigan (managed) switch** + serverda **WiFi karta** bor |
| 6 | Docker qamrovi | **Tarmoq xizmatlari (DHCP/firewall/LB/capture) host'da**, **veb-panel + DB Docker'da** |
| 7 | Autentifikatsiya | **Login+parol (+ ixtiyoriy 2FA) VA IP/MAC cheklovi** — ikki qatlamli |
| 8 | Trafik yozib olish | **On-demand** (admin "Start" bosganda) + **avtomatik rotatsiya** (hajm/vaqt) + **umumiy disk kvotasi** |
| 9 | Guruh IP manzillari | **Har bir server guruhi uchun alohida VIP** (IP alias) — yetarli bo'sh IP bor |
| 10 | Miqyos | **O'rta** — 50–300 qurilma |
| 11 | VPN (masofaviy Wireless LAN) | **WireGuard** |
| 12 | UI/UX | Zamonaviy, minimalist, real-vaqt grafiklar — Claude to'liq dizaynni tanladi |
| 13 | OS | **Ubuntu Server LTS** |
| 14 | Gateway roli | **Ha** — bu server LAN'ning **asosiy shlyuzi (gateway+NAT)**, barcha trafik shu orqali o'tadi |
| 15 | Admin ierarxiyasi | **2 daraja**: Super-admin va oddiy Admin |
| 16 | DDoS chegaralari | **Aqlli standart qiymatlar** (kodda hujjatlashtirilgan) |

**Muhim texnik cheklov:** Claude Code sessiyalari bulutli, izolyatsiyalangan
konteynerda ishlaydi — real LAN, boshqariladigan switch, WiFi karta yoki
root darajasidagi tarmoq huquqlariga ega emas. Shuning uchun kod shu yerda
**yoziladi va imkon qadar sinaladi** (masalan, lokal Postgres bilan API'ni
to'liq end-to-end sinash, Playwright bilan brauzerda UI'ni tekshirish), lekin
DHCP/firewall/WireGuard/packet-capture kabi qismlarning **haqiqiy LAN'dagi
ishlashi faqat real Ubuntu serverda** tekshirilishi mumkin.

---

## 4. Yakuniy arxitektura

```
                         INTERNET
                             │
                    ┌────────▼────────┐
                    │   Bu server:     │
                    │  GATEWAY + NAT   │
                    │  (Ubuntu LTS)    │
                    └────────┬────────┘
        ┌───────────────────┼───────────────────────┐
        │                   │                        │
 ┌──────▼──────┐   ┌────────▼────────┐      ┌────────▼────────┐
 │ Managed     │   │  WiFi karta      │      │  WireGuard VPN  │
 │ Switch      │   │  (hostapd = AP)  │      │  (site-to-site) │
 │ (portlar)   │   │                  │      │                 │
 └──────┬──────┘   └────────┬─────────┘      └────────┬────────┘
        │                   │                          │
   [LAN qurilmalari —   [LAN qurilmalari —      [Uzoqdagi Wireless LAN,
    kabelli]              WiFi]                  internet orqali tunnel]
```

Bitta Ubuntu host ichida **ikki qatlam**, imtiyoz darajasi bo'yicha qat'iy
ajratilgan:

```
DATA-PLANE (root/CAP_NET_ADMIN, systemd, host tarmog'ida):
  dnsmasq (DHCP+DNS) · nftables (firewall) · netdiscd (topilma) ·
  lbd (L4 load balancer) · capd (pcap yozib olish)
        ▲ hammasi mahalliy Unix-socket orqali boshqariladi ▲
CONTROL-PLANE (Docker, imtiyozsiz):
  API server (Go, chi router, JWT auth) ── PostgreSQL (+TimescaleDB, ixtiyoriy)
        │ REST + (kelajakda) WebSocket
  Admin Web Panel (React+TS, faqat adminlarga ochiq, nginx orqali)
```

**Nega shunday bo'lingan:** Privilegiyali ishlarni (paket filtrlash, DHCP, xom
soket, tcpdump) faqat kichik, alohida Go daemon'lar bajaradi (har biri bitta
vazifaga mas'ul). API server esa hech qanday maxsus huquqqa ega emas — u
faqat Postgres bilan va daemon'larning mahalliy (localhost-only) boshqaruv
socketlari bilan gaplashadi. Bu xavfsizlik (hujum yuzasi kichik) va kod
tozaligi uchun.

**Kirish huquqi jadvali:**

| Qurilma turi | LB serverlarga (VIP) | "O'rtadagi" boshqaruv serveriga | Internetga |
|---|---|---|---|
| **User** | ✅ | ❌ | ✅ (administrator xohishiga ko'ra) |
| **Admin** | ✅ | ✅ | ✅ |
| Ro'yxatsiz | ❌ | ❌ | ❌ |

nftables'da ikkita named set (`allowed_user_mac`, `allowed_admin_mac`) orqali
amalga oshiriladi, default policy — **DROP**. `internal/aclsync` (control-plane)
`devices`/`access_grants` jadvalini har 2 soniyada o'qib, `fwctl`ga (data-plane)
push qiladi — **Phase 2, tayyor va real sinaldi**, bo'lim 8'ga qarang.

---

## 5. Texnologiya steki (yakuniy)

| Qatlam | Texnologiya | Sabab |
|---|---|---|
| Data-plane daemonlar | **Go** | Yagona binary, past xotira, systemd bilan integratsiya, root-level tarmoq ishlari uchun standart |
| Control-plane API | **Go** (chi router) | Daemon'lar bilan bitta til — kod bazasi bir xil |
| Ma'lumotlar bazasi | **PostgreSQL 16** (+ TimescaleDB, ixtiyoriy) | Relyatsion + vaqt-qatori ma'lumot bitta DB'da |
| DHCP | **dnsmasq** | Yengil, ishonchli — Phase 3'da haqiqiy DHCP server sifatida sinaldi (lease fayli Phase 1'dan beri o'qiladi) |
| Tarmoq topish | **ARP jadvali + `gosnmp`** (IF-MIB/BRIDGE-MIB) + hostapd control socket | Phase 1'da tayyor, real sinaldi |
| Firewall | **nftables** | Zamonaviy Linux standarti, named sets — Phase 2'da tayyor va real sinaldi |
| L4 Load Balancer | Custom **Go** (`lbd`) | TCP proxy, VIP-per-guruh (hali yozilmagan — Phase 4) |
| Paket ushlash | **libpcap**/tcpdump asosida `capd` | .pcap to'g'ridan-to'g'ri Wireshark'da ochiladi (hali yozilmagan — Phase 6) |
| VPN | **WireGuard** | Eng tez va sodda site-to-site (hali yozilmagan — Phase 7) |
| Frontend | **React + TypeScript + Vite + TailwindCSS** | Zamonaviy, minimalist |
| Grafiklar | **Recharts** + `dataviz` skill palitrasi | Validatsiya qilingan ranglar (`node scripts/validate_palette.js`) |
| Auth | JWT (`golang-jwt/jwt/v5`) + bcrypt (`golang.org/x/crypto`) + TOTP (`pquerna/otp`) | Standart, xavfsiz |
| Deploy | systemd (daemonlar) + Docker Compose (Postgres/API/web) | Foydalanuvchi tanlagan gibrid model |

---

## 6. Ma'lumotlar bazasi sxemasi

`internal/db/migrations/0001_init.up.sql` — barcha jadvallar (Phase 5'da
`0002_backend_metrics.up.sql` bilan `backend_servers.agent_token` va
`backend_metrics` jadvali qo'shildi, quyida ko'rsatilgan):

```
admins(id, username, password_hash, totp_secret, role[super_admin|admin],
       allowed_ip, allowed_mac, created_by, is_active, created_at, last_login_at)

devices(id, mac_address UNIQUE, ip_address, hostname, nickname,
        conn_type[wired|wireless_local|wireless_remote_vpn],
        switch_port_id, ssid, lan_network_id, first_seen_at, last_seen_at, is_online)

access_grants(id, device_id UNIQUE, role[user|admin], granted_by, granted_at)
  -- device_id uchun grant yo'q = bloklangan (default-deny manbasi)

switch_ports(id, switch_name, port_number, label, vlan, link_status, last_change_at)

vpn_peers(id, name, public_key, allowed_subnet, endpoint, is_active, last_handshake_at)

lan_networks(id, name, type, vpn_peer_id, is_active, is_reachable, last_status_check_at)

server_groups(id, nickname, color_hex, vip_address, vip_port, protocol,
              algorithm[round_robin|least_conn], is_active, created_at)

backend_servers(id, group_id, ip, port, weight, is_healthy, last_check_at, response_time_ms,
                 agent_token)  -- Phase 5: cmd/backendagentd shu token bilan o'zini tanitadi

traffic_captures(id, device_id, started_by_admin_id, file_path, started_at,
                  stopped_at, size_bytes, status[recording|rotated|downloaded|error],
                  rotation_reason)

audit_logs(id, actor_admin_id, action, target_type, target_id, details JSONB, created_at)

-- Vaqt-qatori (TimescaleDB hypertable'ga aylantirilishi mumkin, hozircha oddiy jadval):
system_metrics(time, cpu_percent, mem_percent, disk_read_bps, disk_write_bps,
                net_in_bps, net_out_bps, disk_percent)
device_traffic_stats(time, device_id, backend_group_id, bytes_in, bytes_out)
backend_metrics(time, backend_server_id, cpu_percent, mem_percent, disk_percent,
                 disk_read_bps, disk_write_bps, net_in_bps, net_out_bps)
  -- Phase 5: cmd/backendagentd'dan /api/agent/metrics orqali push qilinadi
```

Migratsiyalar `internal/db/migrate.go` orqali **avtomatik** ishga tushadi
(API server startida, `go:embed` bilan o'rnatilgan SQL fayllardan).

---

## 7. Repo tuzilmasi

```
cmd/api/main.go          REST API entrypoint (bootstrap super_admin, migratsiya, HTTP server)
cmd/netdiscd/main.go     netdiscd entrypoint (kollektorlarni ishga tushiradi, socket serveri)
cmd/fwctl/main.go        fwctl entrypoint (deny-all baseline, socket serveri)
cmd/lbd/main.go          lbd entrypoint (LBD_INTERFACE talab qiladi, socket serveri)
cmd/backendagentd/main.go  Backend server metrikasi push-agenti (Phase 5) — gateway'da
                           EMAS, har bir backend serverda ishlaydi, AGENT_API_URL/
                           AGENT_TOKEN talab qiladi
internal/
  config/                 Muhit o'zgaruvchilarini o'qish (.env kabi)
  db/                     Postgres ulanish (pgxpool) + o'rnatilgan migratsiyalar
  models/                 Domen tiplari (Admin, Device, ServerGroup, ...)
  auth/                   JWT, bcrypt, TOTP
  httpapi/                HTTP handlerlar, middleware, router (chi)
  hostmetrics/            CPU/RAM/Disk/Net sampler (gopsutil) — internal/metrics VA
                           cmd/backendagentd ikkalasi ham shu yerdan foydalanadi
  metrics/                Gateway'ning o'z host metrikasini yig'uvchi (hostmetrics ustida)
  netdisc/                netdiscd kollektorlari: arp.go, dnsmasq.go, snmp.go, hostapd.go,
                           store.go (thread-safe in-memory holat), server.go (Unix-socket JSON)
  discovery/              API tomonida: netdiscd snapshot'ini pull qilib Postgres'ga upsert
  firewall/               fwctl: ruleset.go (nft matn generator, pure func, NAT+WAN qattiqlashtirish
                           shu yerda), apply.go (nft -f chaqiradi), gateway.go (ip_forward yoqadi),
                           manager.go (state + serialize), server.go (Unix-socket)
  aclsync/                API tomonida: access_grants'ni Postgres'dan o'qib fwctl'ga push qiluvchi
  lb/                     lbd: types.go (Backend/Group/Status), vip.go (ip addr add/del /32),
                           pool.go (backendState + round_robin/least_conn tanlash), healthcheck.go
                           (davriy TCP-connect probe), proxy.go (accept loop + bidirectional
                           io.Copy), manager.go (VIP'lar bo'yicha reconcile + Sync/Status),
                           server.go (Unix-socket: /sync, /status)
  lbsync/                 API tomonida: server_groups/backend_servers'ni Postgres'dan o'qib
                           lbd'ga push qiluvchi, sog'liqni orqaga yozuvchi
web/                      React + TypeScript + Vite admin paneli
  src/api/                client.ts (fetch wrapper), types.ts
  src/context/            AuthContext (JWT holati)
  src/components/         Layout, DeviceTable, AllDevicesTable (huquq berish), Card/Panel/StatTile
  src/pages/              Dashboard, Server, Users, Admins, Servers, LAN, Firewall, Logs, Login
  src/theme/palette.ts    dataviz skill'dan validatsiya qilingan ranglar
deploy/
  docker/                 api.Dockerfile, web.Dockerfile, docker-compose.yml (control-plane)
  systemd/netdiscd.service  netdiscd uchun tayyor unit fayl
  systemd/fwctl.service   fwctl uchun tayyor unit fayl
  systemd/lbd.service     lbd uchun tayyor unit fayl (LBD_INTERFACE sozlanishi kerak)
  systemd/backendagentd.service  backendagentd uchun tayyor unit fayl (backend serverga
                          o'rnatiladi, gateway'ga emas — AGENT_API_URL/AGENT_TOKEN kerak)
  dnsmasq/p13server.conf.example  Haqiqiy DHCP server uchun tayyor dnsmasq konfiguratsiyasi
  nftables/               Bo'sh — nftables qoidalari kod orqali (internal/firewall) generatsiya
                           qilinadi, statik fayl sifatida saqlanmaydi
docs/deploy.md            To'liq deploy qo'llanmasi (control-plane vs data-plane)
README.md                 Loyiha holati jadvali + tezkor ishga tushirish
```

---

## 8. Hozirgi holat — nima ISHLAYDI, nima yo'q

### ✅ Phase 0 — tayyor va real sinaldi (curl + lokal Postgres + Playwright brauzer testi bilan)

- To'liq DB sxema, migratsiyalar (real Postgres'da sinaldi, up+down ikkalasi ham).
- Auth: login/parol (bcrypt), JWT sessiya, TOTP 2FA tayyor (ixtiyoriy).
- RBAC: super_admin (adminlarni boshqara oladi) vs admin.
- Devices CRUD: nickname berish, User/Admin huquq berish/olish (`access_grants`) —
  **bu default-deny firewall (Phase 2) o'qiydigan yagona haqiqat manbasi**.
- Server groups (load balancing guruhlari): nickname+rang+VIP, backend
  serverlar qo'shish/o'chirish.
- LAN networks: ro'yxat + active/deactive toggle.
- Audit log: har bir admin amali yoziladi.
- **Real** tizim metrikalari (gopsutil): CPU/RAM/Disk/Tarmoq — har 10 soniyada
  yig'ilib, Recharts bilan chiroyli grafik chiziladi (`dataviz` skill
  palitrasi bilan validatsiya qilingan: blue/orange/aqua).
- React admin panel: login sahifasi + 7 bo'limli navigatsiya (Bosh sahifa,
  Server, Userlar, Adminlar, Serverlar, LAN, Firewall, Logs) — barchasi
  real API'ga ulangan, Playwright orqali brauzerda skrinshot qilib
  tekshirilgan, konsolda xato yo'q.
- Ishlab chiqarish jarayonida topib tuzatilgan 2 ta real bug: (1) pgx'ning
  INET tipini `::text` bilan o'qishda `/32` qo'shib yuborishi (`host()`
  funksiyasiga o'tkazildi), (2) metrikalar kollektorida disk hisoblagichi
  har tikda noto'g'ri jamlanib ketishi (grafikda ma'nosiz raqamlar
  chiqargan edi — tuzatildi).

### ✅ Phase 1 — tayyor va real sinaldi (real ARP/dnsmasq/SNMP manbalar bilan, unit test + end-to-end)

- **`netdiscd`** (`cmd/netdiscd`, `internal/netdisc/`) — imtiyozsiz Postgres bilan
  ishlamaydigan alohida daemon (data-plane), 4 ta mustaqil, bir-biridan
  qat'iy nazar ishlaydigan kollektor bilan:
  - **ARP** (`/proc/net/arp`) — passiv, real vaqtda "hozir shu yerdami" signali.
  - **dnsmasq lease fayli** — hostname va fallback IP.
  - **SNMP** (IF-MIB port holati + BRIDGE-MIB `dot1dTpFdbTable` orqali
    MAC→port, ikkinchisi best-effort) — real `snmpd` instansiyasiga qarshi
    sinaldi.
  - **hostapd control socket** — lokal WiFi mijozlari (protokol yozilgan,
    real hostapd'siz to'liq sinalmagan — quyidagi cheklovga qarang).
  - Har bir manba ixtiyoriy: birortasi sozlanmagan/mavjud bo'lmasa, shunchaki
    o'tkazib yuboriladi (masalan managed switch yo'q bo'lsa, faqat ARP ishlaydi).
  - Natija **hech qachon o'chirilmaydi** — qurilma "offlayn" bo'lib qoladi
    (`NETDISC_STALE_AFTER`, standart 90s), lekin ro'yxatdan yo'qolmaydi.
  - Snapshot `/run/p13server/netdiscd.sock` orqali JSON (`GET /snapshot`)
    sifatida beriladi — netdiscd Postgres haqida umuman bilmaydi.
- **`internal/discovery`** (control-plane, `cmd/api` ichida ishlaydi) — shu
  socketni har 5 soniyada so'rab, `devices` va `switch_ports` jadvallariga
  upsert qiladi (tranzaksiya ichida, switch port ID'larini avval yechib
  keyin device'larga bog'laydi). netdiscd ishlamasa, bir marta ogohlantirib
  jim davom etadi (API'ni qulatmaydi).
- **LAN sahifasiga qo'shildi:** "Barcha aniqlangan qurilmalar" jadvali —
  admin endi har qanday topilgan qurilmaga (nickname + User/Admin/Yo'q
  huquq) to'g'ridan-to'g'ri shu yerdan belgilay oladi. Bu Phase 0'dagi
  bo'shliqni to'ldirdi: avval Users/Admins sahifalari faqat **allaqachon**
  huquq berilgan qurilmalarni ko'rsatardi, huquq berishning o'zi uchun
  panelda hech qanday yo'l yo'q edi.
- **Sinov:** unit testlar (`internal/netdisc/*_test.go` — ARP parser, dnsmasq
  parser, SNMP OID/MAC ajratish), va **to'liq end-to-end** integratsion sinov:
  real `/proc/net/arp` + qo'lda yozilgan dnsmasq lease fayli + sandbox'da
  ishga tushirilgan haqiqiy `snmpd` (IF-MIB) → netdiscd → Unix socket →
  `internal/discovery` → Postgres → REST API → React LAN sahifasi →
  Playwright orqali huquq berish → Userlar sahifasida darhol ko'rinishi —
  hammasi haqiqiy ma'lumot bilan tekshirildi.
- **Bilingan cheklovlar (halol, kodda ham yozilgan):**
  - hostapd real WiFi uskunasisiz to'liq sinalmadi (protokol client kodi
    yozilgan, lekin bu sandbox'da wireless PHY yo'q).
  - SNMP orqali MAC→port xaritalash (`dot1dTpFdbTable`) faqat switch shu
    jadvalni qo'llab-quvvatlasa ishlaydi — sinovda ishlatilgan oddiy
    net-snmp agenti buni bermaydi (kutilgan holat, kodda hujjatlashtirilgan).
  - Faol ARP probing (arping) yo'q — faqat passiv, kernel allaqachon
    yechgan yozuvlarni o'qiydi (`CAP_NET_RAW` talab qilinishi sababli
    Phase 1 doirasidan tashqarida qoldirildi).
  - "Wireless remote VPN" turidagi qurilmalar (Phase 7, masofaviy LAN)
    netdiscd tomonidan aniqlanmaydi — bu alohida VPN peer'ning o'z LAN'i,
    kelajakda alohida mexanizm kerak bo'ladi.

### ✅ Phase 2 — tayyor va real sinaldi (nftables + `ip netns` orqali haqiqiy trafik bilan)

- **`fwctl`** (`cmd/fwctl`, `internal/firewall/`) — Postgres'ga umuman ulanmaydigan
  daemon (netdiscd bilan bir xil printsip, lekin teskari yo'nalishda: control-plane
  MA'LUMOTNI fwctl'ga **push** qiladi, undan pull qilmaydi):
  - Ishga tushganda darhol **deny-all baseline**'ni yuklaydi (`ApplyBaseline`) —
    control-plane'dan birinchi sync kelmaguncha ham, fwctl qulab tushib qayta
    ishga tushsa ham, standart holat har doim "hech kim kirolmaydi", hech qachon
    "qoidalar yo'q = hammaga ochiq" emas.
  - `/run/p13server/fwctl.sock` orqali `POST /sync {user_macs, admin_macs}` va
    `GET /status` qabul qiladi.
  - Har bir sync'da **butun nftables jadvalini atomik ravishda** (`nft -f -`,
    bitta kernel tranzaksiyasi) qayta quradi: `add table` → `flush table` →
    setlarni e'lon qilish+flush qilish → elementlarni qo'shish → zanjir/qoidalarni
    qayta yozish. Bu "diff qilib qo'shish/o'chirish" emas, **to'liq almashtirish** —
    hech qachon eski va yangi holat aralashib qolmaydi.
  - 2 ta zanjir: `lan_forward` (`forward` hook) — faqat `allowed_user_mac` yoki
    `allowed_admin_mac`dagi qurilmalar forward qilinadi; `management_input`
    (`input` hook) — faqat `allowed_admin_mac` `FWCTL_MANAGEMENT_PORTS`ga
    (standart `22,8080`) kira oladi. Ikkalasi ham `policy drop`.
  - DDoS baseline (qaror #16): ICMP echo-request rate-limit, va har bir
    admin/user MAC (yoki boshqaruv porti uchun har bir source IP) uchun
    alohida **dinamik meter** orqali yangi-ulanish tezligi cheklanadi —
    aniq sonlar va sabablari `internal/firewall/ruleset.go`da comment
    sifatida yozilgan.
- **`internal/aclsync`** (control-plane, `cmd/api` ichida) — `devices`+
  `access_grants`ni har 2 soniyada o'qib, MAC ro'yxatlarini fwctl'ga push
  qiladi. fwctl ishlamasa, bir marta ogohlantirib jim davom etadi.
- **`GET /api/firewall/status`** qo'shildi — Firewall sahifasi endi ikkita
  holatni yonma-yon ko'rsatadi: DB'dagi "nima bo'lishi kerak" (access_grants)
  va fwctl'dan kelgan "hozir kernelda nima yuklangan" (haqiqiy holat) — ikkisi
  farq qilsa, bu sinxronizatsiya kechikishi yoki fwctl ulanmaganini bildiradi.
- **Sinov — bu safar unit testdan ham uzoqroqqa borildi:** `ip netns` orqali
  butunlay izolyatsiyalangan 3 ta tarmoq nomlar maydoni qurildi (`lan` qurilma
  ↔ `gw`, haqiqiy fwctl+nftables ishlaydigan ↔ `backend` nishon), va **haqiqiy
  `curl` trafigi** bilan butun kirish matritsasi tekshirildi: ro'yxatsiz →
  bloklangan (forward HAM input HAM), user → forward ochiq/input yopiq, admin →
  ikkalasi ochiq, **bekor qilish → yana bloklangan**, qayta berish → yana ochiq.
  Keyin xuddi shu zanjir **haqiqiy Postgres orqali** (`internal/aclsync`ning
  o'zi, qo'lda curl qilmasdan) ham qayta tasdiqlandi.
- **Shu qattiq sinov davomida 2 ta real nftables xatosi topilib tuzatildi**
  (ikkalasi ham endi kodda va shu yerda hujjatlashtirilgan, chunki hech qanday
  qo'llanma/AI training bu ikkisini aniq aytmaydi):
  1. Inline `meter name { ... }` sintaksisi meter'ni birinchi marta yashirin
     yaratadi, lekin `flush table` uni o'chirmaydi — keyingi reconciliation
     "File exists" xatosi bilan qulaydi. Yechim: meter'larni oldindan
     `add set ... { flags dynamic; }` bilan e'lon qilib, qoidada
     `update @name { ... }` orqali ishlatish.
  2. **`flush table` mavjud named set'larning elementlarini tozalamaydi**
     (faqat zanjir/qoidalarni) — bu spetsifikatsiyada yoki odatiy hujjatlarda
     aniq aytilmagan, xulq-atvor sinov orqali aniqlangan. Buning oqibati juda
     jiddiy edi: **bekor qilingan (revoke) qurilma MAC'i hech qachon
     `allowed_user_mac`/`allowed_admin_mac`dan chiqmasdi** — ya'ni huquqni
     olib tashlash ishlamas, qurilma abadiy kira olaverar edi. Har bir set
     uchun alohida `flush set` qo'shish orqali tuzatildi va bu holat uchun
     maxsus regressiya testi (`TestBuildRuleset_RevocationFlushesSetEvenWithNoRemainingElements`)
     yozildi.
- **Bilingan cheklovlar:** DDoS himoyasi faqat bitta gateway darajasidagi
  "aqlli baseline" — haqiqiy distributed hujumga bardosh berish uchun
  yuqori oqimda (upstream) scrubbing kerak, bu doirasidan tashqarida.
  `lbd` (Phase 4) hali yo'qligi sababli hozircha `lan_forward`dagi "ruxsat
  berilgan" trafik biror haqiqiy load-balancing serverga emas, faqat
  gateway orqali umuman forward qilinishga (Phase 3'dan keyin — internetga
  ham) ruxsat beradi.

### ✅ Phase 3 — tayyor va real sinaldi (haqiqiy dnsmasq DHCP + NAT + `ip netns`)

- **DHCP** — alohida Go daemon yozilmadi (qaror #4: tayyor vositalar
  ustida qurish): `deploy/dnsmasq/p13server.conf.example` — haqiqiy
  `dnsmasq` paketini LAN interfeysida DHCP server sifatida ishga
  tushiradigan, hujjatlashtirilgan konfiguratsiya namunasi. Lease fayli
  yo'li Phase 1'dagi `netdiscd` allaqachon o'qiydigan yo'l bilan bir xil —
  hech narsa qayta ulanmaydi.
- **NAT + gateway qattiqlashtirish** — `internal/firewall`ga qo'shildi
  (`RulesetConfig.WANInterface`):
  - `EnableIPForwarding()` (`internal/firewall/gateway.go`) — fwctl
    ishga tushganda `net.ipv4.ip_forward=1`ni o'rnatadi (buning
    o'zisiz kernel paketlarni forward zanjiriga umuman yubormaydi).
  - `nat_postrouting` zanjiri (`type nat hook postrouting`) — WAN
    interfeysidan chiqayotgan trafikni masquerade qiladi.
  - **Har bir** MAC-ruxsat qoidasiga (`lan_forward` HAM
    `management_input`da) `iifname != <wan>` qo'shildi — MAC ro'yxati
    yolg'iz o'zi WAN tomonidan kelayotgan soxta MAC'ni to'xtata olmaydi;
    bu qoida "o'rtadagi server" internetdan **hech qachon** ochilmasligini
    kafolatlaydi.
- **Sinov — Phase 1/2'dan ham bir qadam oldinga:** `lan`↔`gw`↔`wan` (3
  namespace) topologiyasi qurilib, `gw`da **haqiqiy `dnsmasq`** DHCP
  server sifatida ishga tushirildi. `lan`dagi qurilma **haqiqiy
  `udhcpc`** orqali IP oldi (real DISCOVER/OFFER/REQUEST/ACK almashinuvi),
  va lease fayli aynan Phase 1 parseri kutgan formatda chiqdi. Keyin butun
  kirish+NAT matritsasi haqiqiy `curl` trafigi bilan tekshirildi:
  ro'yxatsiz → bloklangan, user → internetga chiqadi (NAT bilan) lekin
  boshqaruv portiga yo'q, admin → ikkalasi ham bor, **bekor qilish →
  yana bloklangan**. NAT ishlashi ikki mustaqil usulda tasdiqlandi:
  conntrack jadvalida va — eng ishonchlisi — "internet" serverining o'z
  HTTP access logida: har bir so'rov LAN mijozining haqiqiy IP'si
  (`10.0.1.81`) emas, **gateway'ning WAN IP'si** (`203.0.113.1`) sifatida
  qayd etildi. WAN-tomon qattiqlashtirish alohida tekshirildi: `wan`
  namespace'ning interfeys MAC manzili ataylab ruxsat berilgan admin
  MAC'iga o'zgartirildi va shunda ham boshqaruv portiga kirish bloklanishi
  tasdiqlandi — ya'ni bu himoya haqiqatan `iifname`ga tayanadi, faqat
  MAC to'plamiga emas.
- **Bilingan cheklovlar:** WAN interfeysi orqali real internetga ulanish
  (masalan PPPoE, DHCP-client WAN tomonida) sinalmadi — bu sof Linux
  tarmoq konfiguratsiyasi masalasi, fwctl/nftables mantig'iga aloqasi yo'q.
  hostapd'ning DHCP integratsiyasi (WiFi mijozlariga IP berish) alohida
  ko'rib chiqilmadi — odatda bitta dnsmasq bir nechta interfeysga xizmat
  qila oladi, konfiguratsiya namunasida eslatilgan.

### ✅ Phase 4 — tayyor va real sinaldi (haqiqiy L4 TCP load balancer + `ip netns`)

- **Arxitektura** (`internal/lb/`): `Manager` — bir host'da ishlayotgan
  barcha VIP'larni boshqaradi, `netdisc.Store`/`firewall.Manager` kabi
  hech qachon Postgres'ga ulanmaydi. Har bir guruh uchun:
  - `vip.go` — VIP manzilini alohida dummy interfeys emas, `LBD_INTERFACE`
    (LAN interfeysi)ga `/32` ikkilamchi manzil sifatida qo'shadi
    (`ip addr add <vip>/32 dev <iface>`) — qaror: kernel bu manzil uchun
    ARP'ga oddiy host kabi javob beradi, LAN mijozlari uni tarmoqdagi
    boshqa har qanday host kabi ko'radi.
  - `proxy.go` — `net.Listen("tcp", vip:port)`, har bir qabul qilingan
    ulanish uchun backend tanlanadi va ikkala tomonlama `io.Copy` bilan
    proksi qilinadi (L4, protokolga bog'liq emas).
  - `pool.go` — `round_robin` (atomik counter) va `least_conn` (eng kam
    faol ulanishli backend'ni skanerlash) tanlash algoritmlari,
    2-ketma-ket-xato/1-muvaffaqiyat flap-oldini olish chegarasi bilan.
  - `healthcheck.go` — har 3 soniyada oddiy TCP-connect probe (HTTP shart
    emas — "istalgan turdagi server" talabiga mos, protokolga bog'liq
    bo'lmagan tekshiruv).
  - `manager.go` — `Sync(groups)` xohlangan holatni joriy holat bilan
    solishtiradi: yangi guruhlarga VIP+listener ochadi, o'chganlarga
    VIP'ni bo'shatadi, o'zgarganlarga esa `backendState`ni addr bo'yicha
    qayta ishlatib (health/ulanish tarixini yo'qotmasdan) pool'ni
    almashtiradi.
  - `server.go` — Unix-socket: `POST /sync` (xohlangan guruhlar),
    `GET /status` (har bir backend'ning `is_healthy`/`response_time_ms`/
    `active_conns`'i).
- **Control-plane** (`internal/lbsync/`) — `aclsync`/`discovery` bilan bir
  xil naqsh: har ~3s `server_groups`(`is_active = true`)/`backend_servers`ni
  o'qib lbd'ga `/sync` orqali push qiladi, `/status`ni pull qilib
  `backend_servers.is_healthy`/`last_check_at`/`response_time_ms`ga
  yozadi. lbd ishlamasa — bir marta ogohlantirib, urinishda davom etadi
  (API'ni yiqitmaydi).
- **Topilgan haqiqiy bag** (endi `internal/lb/manager.go`da yuklama
  ko'taruvchi izoh): `Manager.startLocked` VIP listener va health-check
  goroutine'larining umrini `Sync(ctx, ...)`ning `ctx` parametridan olar
  edi. `ServeControl`ning `/sync` handleri `m.Sync(r.Context(), ...)`
  chaqiradi, `net/http` esa HTTP javobi yozilgan zahoti `r.Context()`ni
  bekor qiladi — natijada **har bir muvaffaqiyatli sync'dan keyin VIP
  listener darhol o'zini yopib qo'yar edi**. Simptom: `/sync` 200
  qaytaradi, `ip addr show` VIP manzilini to'g'ri ko'rsatadi, ARP VIP'ni
  gateway MAC'iga to'g'ri hal qiladi (`ip neigh show` → `REACHABLE`), lekin
  `ss -tln` portda **hech qanday listener yo'qligini** ko'rsatadi va
  `curl` "connection refused" beradi. Tuzatish: `Manager`ga alohida,
  daemon umri bilan yashaydigan `baseCtx` maydoni qo'shildi
  (`NewManager(iface, baseCtx)`, `cmd/lbd/main.go`da `signal.NotifyContext`
  natijasi beriladi); listener/health-check goroutine'lari endi shu
  `baseCtx`dan, sync chaqiruvining request-scoped `ctx`sidan emas,
  hosil qilinadi (u faqat sinxron `ip addr add/del` chaqiruvlari uchun
  ishlatiladi).
- **Sinov — 4-tugunli `ip netns` topologiyasi** (`lan` mijoz ↔ `gw`
  haqiqiy `lbd` bilan ↔ `backend1`/`backend2`, har biri haqiqiy
  `python3 -m http.server`):
  1. VIP `10.0.1.100:80`, `round_robin`, backend'lar `10.0.2.2:80` va
     `10.0.3.2:80` bilan sync qilindi. ARP: `ip neigh show` → `REACHABLE`.
     6 ketma-ket haqiqiy `curl` so'rovi mukammal almashdi:
     `BACKEND1-OK/BACKEND2-OK` × 3.
  2. `backend1`ning http.server jarayoni o'chirildi. ~2 health-check
     tsiklidan (6s) so'ng `GET /status` uni `is_healthy: false` deb
     ko'rsatdi. **Muhimi** — bu faqat yozilgan holat emasligini
     tasdiqlash uchun yashab turgan listener orqali yana 6 ta haqiqiy
     `curl` yuborildi: barcha 6tasi ham faqat `BACKEND2-OK` qaytardi —
     ya'ni `pool.pick()` haqiqatan ham nosog'lom backend'ni jonli
     trafikdan chetlashtiradi, shunchaki holatni yozib qo'yib qolmaydi.
  3. `backend1` qayta ishga tushirildi, keyingi muvaffaqiyatli probe'dan
     so'ng `GET /status` uni yana `is_healthy: true` qildi, va jonli
     trafik yana `BACKEND1-OK/BACKEND2-OK` almashinishiga qaytdi.
  4. Guruh o'chirildi (`{"groups": []}` sync) — VIP manzili
     (`ip addr show`) va listener (`ss -tln`) ikkalasi ham yo'qoldi, mijoz
     endi ulana olmadi (`connection refused`).
  5. `least_conn`: guruh qayta `least_conn` bilan ochilib, 4 ta uzoq
     ulanish (`exec 3<>/dev/tcp/vip/80`) ketma-ket ochildi va har birida
     `GET /status`dagi `active_conns` tekshirildi: `1-0 → 1-1 → 2-1 →
     2-2` — har doim eng kam yuklangan backend tanlandi, hech qachon
     tengsiz taqsimlanmadi.
  6. **Butun boshqaruv zanjiri haqiqiy Postgres bilan**: `server_groups`/
     `backend_servers`ga haqiqiy qatorlar yozildi, haqiqiy `cmd/api`
     ishga tushirildi (`internal/lbsync.Run` bilan), va u avtomatik lbd'ga
     push qilib, `backend_servers.is_healthy`/`last_check_at`/
     `response_time_ms`ni orqaga yozganini tasdiqladi. `is_active = false`
     qo'yilganda keyingi tsiklda VIP o'chdi, qayta `true` qilinganda
     qayta ochildi va trafik xizmat qila boshladi — lbd'ning o'zi hech
     qachon Postgres'ga tegmadi.
- **UI bo'shlig'i topildi va tuzatildi:** `ServersPage.tsx` `is_healthy`ni
  ko'rsatardi, lekin `false`ni "hali tekshirilmagan" va "tekshirilib,
  ishlamayapti" holatlaridan ajrata olmasdi (ikkalasi ham bir xil kulrang
  "○ Tekshirilmagan" bilan chizilardi) — Phase 4dan keyin bu farq endi
  haqiqiy va muhim. `last_check_at`ning borligiga qarab uch holatga
  bo'lindi: hali tekshirilmagan (kulrang), sog'lom (yashil), ishlamayapti
  (qizil) — va `response_time_ms` ustuni qo'shildi.
- **Bilingan cheklovlar:** vazn (`weight`) hozircha faqat saqlanadi va
  ko'rsatiladi — tanlash algoritmlari hali og'irlik bo'yicha emas, teng
  ravishda (yoki eng kam ulanish bo'yicha) tanlaydi. UDP backend'lar
  qo'llab-quvvatlanmaydi (L4 TCP-only, talabga mos).

### ✅ Phase 5 — tayyor va real sinaldi (haqiqiy Postgres + real brauzer bilan)

- **Qaror (foydalanuvchidan so'ralgan):** backend serverlar metrikasi 3 ta
  variant orasidan (yengil Go push-agent / SNMP / SSH) **yengil Go
  push-agent**ni tanladi — loyihaning boshqa hamma joyida ishlatilgan
  "Go daemon + push" uslubiga mos, backend qanday xizmat/OS ishlatishidan
  qat'iy nazar ishlaydi.
- **Arxitektura — bu repo'dagi boshqa hamma daemon'dan farqli:**
  `cmd/backendagentd` gateway hostida EMAS, load balancing (Phase 4) VIP'i
  orqasidagi **har bir backend serverning o'zida** ishlaydi. Shu sababli
  mahalliy Unix-socket emas, tarmoq orqali HTTP bilan control-plane
  API'ga ulanadi (`AGENT_API_URL`) — netdiscd/fwctl/lbd'dan farqli, chunki
  ular bilan bir xil host'da control-plane yo'q.
  - `internal/hostmetrics` (yangi, umumiy paket) — CPU/RAM/disk/tarmoq
    o'lchash mantig'i (gopsutil) `internal/metrics`dan (gateway'ning o'z
    "Server" bo'limi) shu yerga ko'chirildi, ikkalasi ham (`cmd/api`ning
    o'z-metrikasi VA `cmd/backendagentd`) endi shu bitta `Sampler`dan
    foydalanadi — mantiq ikki marta yozilmagan.
  - Har bir backend qo'shilganda (`handleAddBackend`) tasodifiy 32-baytli
    `agent_token` avtomatik yaratiladi (`crypto/rand`) va
    `backend_servers.agent_token`ga yoziladi — bu login-parol emas,
    mashina-mashina bearer credential, shuning uchun (JWT_SECRET yoki
    socket yo'llari kabi) ochiq matnda saqlanadi va Serverlar sahifasida
    doim ko'rinadi (bcrypt bilan xeshlanmagan — qayta ko'rish/nusxalash
    kerak bo'ladigan API-kalit, parol emas).
  - `POST /api/agent/metrics` — `internal/httpapi`da atayin JWT
    `requireAuth` guruhidan **tashqarida**: chaqiruvchi tizimga kirgan
    admin emas, tarmoqdagi boshqa mashina, shuning uchun
    `Authorization: Bearer <agent_token>` to'g'ridan-to'g'ri
    `backend_servers.agent_token`ga solishtiriladi. Muvaffaqiyatli bo'lsa
    natija yangi `backend_metrics` jadvaliga yoziladi.
  - `POST /api/backends/{id}/regenerate-token` — token yo'qolgan/oshkor
    bo'lgan holatda, backend'ni o'chirib-qayta yaratmasdan (bu uning
    sog'liq tarixini yo'qotardi) yangi token chiqaradi, eskisi darhol
    ishlamay qoladi.
  - `GET /api/backends/{id}/metrics` — Serverlar sahifasining har bir
    backend qatoridagi "Metrikalar" tugmasi bosilganda ochiladigan
    kengaytirilgan panel uchun so'nggi N o'lchovni qaytaradi.
- **Sinov — haqiqiy Postgres + haqiqiy ikkita binary + haqiqiy brauzer:**
  real `cmd/api` (mahalliy Postgres'ga ulangan) va real `cmd/backendagentd`
  ishga tushirilib, real API orqali yaratilgan haqiqiy token bilan
  ulandi — 2 soniyalik intervalda haqiqiy gopsutil o'lchovlari
  `backend_metrics`ga tushganini ham to'g'ridan-to'g'ri SQL bilan, ham
  `GET /api/backends/{id}/metrics` orqali tasdiqladi. Autentifikatsiya 3
  usulda tekshirildi: noto'g'ri token → 401, `Authorization` header'siz →
  401, `regenerate-token` chaqirilgach eski token darhol 401 bera
  boshladi (yangi token esa 200) — `cmd/api`ni qayta ishga tushirmasdan.
  Agent noto'g'ri/bekor qilingan token bilan qulamadi — bir marta
  ogohlantirib, urinishda davom etdi (aclsync/discovery/lbsync bilan bir
  xil chidamlilik naqshi). Serverlar sahifasining o'zi **haqiqiy headless
  brauzerda** (Playwright, `chromium`) tekshirildi: token ko'rinishi va
  "Nusxalash" tugmasi orqali clipboard'ga to'g'ri nusxalanishi, "Tokenni
  yangilash" tugmasi tokenni haqiqatan almashtirishi, va agent ishga
  tushgandan keyin jonli CPU/RAM/Disk grafigi haqiqiy ma'lumot bilan
  chizilishi — hammasi brauzer konsolida bironta xatosiz.
- **Shu sinov davomida Phase 4'ning o'zidan qolgan eskirgan ma'lumot
  bug'i topildi va tuzatildi** (Phase 5 kodining o'z xatosi emas): Phase
  4'ning `ip netns` sinovi paytida bitta backend qatorining `ip`/`port`i
  vaqtincha boshqa manzilga o'zgartirilib, keyin asl holatiga
  qaytarilgan edi, lekin `is_healthy`/`last_check_at`/`response_time_ms`
  hech qachon tozalanmagan edi — natijada Serverlar sahifasi bu
  backend'ni hech kim tekshirmagan haqiqiy manzili uchun "Sog'lom" deb
  ko'rsatib turardi. Aynan Phase 4'ning o'zida qilingan UI tuzatishi
  ("tekshirilmagan" va "tekshirilib ishlamayapti"ni ajratish) shu holatni
  yashirmay ko'rinadigan qildi — shu orqali topildi. Uchta ustunni haqiqiy
  "hali tekshirilmagan" holatiga (`false`/`NULL`/`NULL`) qaytarish bilan
  tuzatildi.
- **Bilingan cheklov, hal qilinmagan (yashirilmagan):** `fwctl`ning
  `management_input` zanjiri (Phase 2) gateway hostidagi boshqaruv
  portlariga faqat admin-MAC qurilmalarga ruxsat beradi. Agar backend
  server aynan shu fwctl nazorat qiladigan LAN'da tursa (alohida
  server/boshqaruv tarmog'ida emas), uning agent push'lari **shu firewall
  tomonidan jimgina bloklanadi** — MAC'iga admin huquqi berilmasa. Bu
  muammoni hal qiluvchi alohida "faqat metrika uchun" nftables ruxsati
  hali yozilmagan; hozircha backend serverlarni fwctl nazorat qilmaydigan
  tarmoqqa joylashtirish yoki (kelishilgan holda) MAC'iga admin huquqi
  berish kerak bo'ladi.

### ⏳ Hali yozilmagan (har sahifada halol "Phase X'da qo'shiladi" deb yozilgan, soxta ma'lumot yo'q)

| Bosqich | Nima | Fayllar (hali yo'q) |
|---|---|---|
| Phase 6 | `capd` — on-demand pcap yozib olish, rotatsiya, kvota | `cmd/capd/` |
| Phase 7 | WireGuard site-to-site (masofaviy Wireless LAN), real-vaqt reachability | — |
| Phase 8 | RBAC'ning API bo'ylab to'liq qo'llanilishi, xavfsizlik audit, dizayn siyqallashtirish | — |

**Muhim:** `cmd/capd` papkasi hozircha repo'da yo'q (bo'sh papkalar
git'da saqlanmaydi) — Phase 6 boshlanganda yaratiladi.

---

## 9. Git tarixi va branch holati (MUHIM — chalkashmaslik uchun)

Bu loyiha ustida **bir nechta parallel Claude Code sessiyasi** ishlagan va bu
katta chalkashlikka olib keldi. Hozirgi (to'g'ri) holatni bilib olish uchun:

- **`master`** — repo'ning **haqiqiy default branchi**. **Hozir shu yerda
  yuqorida tasvirlangan Phase 0 loyihasi bor** (Go API + React panel).
  Bu yerga to'g'ridan-to'g'ri push qilingan (PR orqali emas — branch
  himoyasi yo'q edi).
- **`main`** — boshqa bir (eski) sessiya ishlatgan branch. Hozir **bo'sh**
  (loyiha bu yerdan butunlay o'chirilgan, foydalanuvchi so'rovi bilan).
  **Eskirgan, ishlatilmaydi.**
- **`claude/busy-albattani-7yi10r`** — ushbu suhbat ishlagan branch. Hozir
  **bo'sh** (`main`ga mos). **Eskirgan, ishlatilmaydi.**
- Boshqa (notanish) sessiyalardan qolgan, `master`ga qaratilgan **PR #3** va
  **PR #4** ochiq turibdi ("Add a scripted failover check", "Document the
  p1-3server setup as a markdown spec") — bu suhbat bilan bog'liq emas,
  ehtiyot bo'lib ko'rib chiqish kerak (ehtimol endi kerak emas, chunki
  `master` butunlay yangi arxitekturaga o'tgan).
- `master`da bizning ishimizdan mustaqil ravishda paydo bo'lgan oraliq
  commit (`f1f6d65 "change"`) bor edi — eski nginx demo'ni Admin/Reader/User
  portlar bilan RBAC'ga o'xshatishga urinish (primitiv, bizning
  arxitekturamizga mos kelmaydi). **Bu commit endi `master`da yo'q** — Phase 0
  uni ham eski demo bilan birga almashtirdi.

**Xulosa: bundan buyon `master` — yagona haqiqat manbai.** Yangi ish shu
branch (yoki undan olingan branch) ustida davom etishi kerak.

---

## 10. Ishga tushirish (development)

```bash
# 1) Postgres (lokal)
sudo -u postgres psql -c "CREATE ROLE p13server WITH LOGIN PASSWORD 'devpassword';"
sudo -u postgres psql -c "CREATE DATABASE p13server OWNER p13server;"

# 2) API
export DATABASE_URL='postgres://p13server:devpassword@127.0.0.1:5432/p13server?sslmode=disable'
export JWT_SECRET=$(openssl rand -hex 32)
export BOOTSTRAP_ADMIN_USERNAME=superadmin
export BOOTSTRAP_ADMIN_PASSWORD='ChangeMe123!'
go run ./cmd/api

# 3) Frontend (boshqa terminalda)
cd web && npm install && npm run dev
```

`http://localhost:5173` → yuqoridagi login/parol bilan kirish. Production
deploy (Docker Compose + systemd) uchun: `docs/deploy.md`.

---

## 11. Muhim texnik qarorlar va sabablari (kelajakda savol tug'ilmasligi uchun)

- **Nega Go ham daemon, ham API uchun?** — Bitta til, bitta kod bazasi,
  daemon'lar va API o'rtasida struct/proto ta'riflarini bo'lishish oson.
- **Nega API'ni ham Docker'da, daemon'larni host'da?** — API'ga hech qanday
  maxsus tarmoq huquqi kerak emas (faqat Postgres + mahalliy socket), lekin
  DHCP/firewall/packet-capture'ga root/`CAP_NET_ADMIN` va host tarmog'i
  kerak. Imtiyozni minimal saqlash — xavfsizlik uchun.
- **Nega `access_grants` alohida jadval, `devices`ning o'zida emas?** —
  "Grant yo'q = bloklangan" qoidasini tabiiy ravishda ifodalaydi (default-deny),
  va `fwctl` shu jadvalni to'g'ridan-to'g'ri nftables setlariga aylantiradi.
  Butun kirish-huquqi tizimining yagona haqiqat manbai shu jadval. 2-band
  ("o'rtadagi server nima") va 14-band (gateway roli) qarorlari bilan birga
  o'qing — bular birgalikda butun firewall mantig'ini belgilaydi.
- **Nega server guruhlari uchun alohida VIP (IP alias), port emas?** — 9-band
  qaroriga ko'ra yetarli bo'sh IP bor; bu userlar tomonidan "bitta guruh =
  bitta IP" tarzida ko'rinishini soddalashtiradi (spetsifikatsiyaning aynan
  o'zi shunday so'ragan).
- **Nega pcap yozib olish on-demand, doimiy emas?** — 8-band qaroriga ko'ra;
  standart holatda hech kimning trafigi yozilmaydi, faqat admin aniq bir
  userni tekshirish uchun "Start" bosganda boshlanadi — maxfiylik va disk
  joyini tejash uchun.
- **Nega TimescaleDB emas, oddiy Postgres jadvali?** — Development muhitida
  TimescaleDB kengaytmasi mavjud emas edi; sxema shunday yozilganki,
  production'da `SELECT create_hypertable(...)` chaqirilsa, hech narsani
  o'zgartirmasdan hypertable'ga aylanadi.
- **Nega `dataviz` skill palitrasidan blue/orange/aqua?** — Validatsiya
  skripti (`validate_palette.js`) bilan tekshirilgan, CVD (rangni farqlay
  olmaslik) xavfsiz uchlik — grafiklarda 3 ta parametr (CPU/RAM/Disk yoki
  Net in/out) bir vaqtda ko'rsatilganda foydalanildi.

---

## 12. Keyingi qadam

Phase 1 (`netdiscd`), Phase 2 (`fwctl`), Phase 3 (Gateway/DHCP/NAT),
Phase 4 (`lbd`) va Phase 5 (`backendagentd`) tayyor va real sinaldi —
bo'lim 8'ga qarang. Bu server endi to'liq ishlaydigan LAN gateway **va**
load balancer: DHCP beradi, kirish huquqini nazorat qiladi, ruxsat
berilganlarni internetga NAT bilan chiqaradi, `lan_forward`dagi ruxsat
berilgan trafikni haqiqiy VIP'lar orqali orqadagi serverlarga taqsimlaydi,
va endi har bir backend serverning o'z host metrikasi (CPU/RAM/disk/
tarmoq) ham ko'rinadi. Navbatdagi ish — **Phase 6: `capd`**
(`cmd/capd/`) — on-demand pcap yozib olish: admin Userlar sahifasida bitta
qurilma uchun "Start" bosganda uning trafigini `.pcap` formatida yozib
boshlaydi (rotatsiya hajm/vaqt bo'yicha, umumiy disk kvotasi bilan — qaror
#8), fayl Logs bo'limiga tushadi, "Download" bosilganda vaqtincha
to'xtab, fayl yakunlanib yuklanadi, so'ng yangi faylga yozish davom etadi
(bo'lim 2.3'dagi Logs talabi). `capd` ham netdiscd/fwctl/lbd kabi
Postgres'ga bevosita ulanmasligi kerak — `libpcap`/tcpdump asosida, root/
`CAP_NET_RAW` talab qiladi. Har bosqich tugagach ushbu faylni va
`README.md`/`docs/deploy.md`ni yangilab borish tavsiya etiladi.
