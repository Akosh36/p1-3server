# LAN Access & Load-Balancing Control Platform

Zabbix'ga o'xshash, lekin soddalashtirilgan: bitta LAN tarmog'ini boshqaruvchi, ikki
darajali (User/Admin) kirish huquqini MAC/IP asosida nazorat qiluvchi, load
balancing qiladigan va admin uchun real-vaqt monitoring paneli beruvchi platforma.

Loyihaning to'liq dizayn qarorlari va sabablari uchun: **[docs/deploy.md](docs/deploy.md)**.

## Arxitektura (qisqacha)

Bitta Ubuntu-server LAN'ning shlyuzi (gateway+NAT) bo'ladi va ikki qatlamga bo'lingan:

- **Control-plane** (imtiyozsiz): PostgreSQL + REST API (`cmd/api`, Go) + React admin
  panel (`web/`). Docker'da ishlaydi.
- **Data-plane** (root/`CAP_NET_ADMIN`, host tarmog'ida): `netdiscd` (qurilma topish),
  `fwctl` (nftables ACL), `lbd` (L4 load balancer), `capd` (pcap yozib olish).
  systemd orqali to'g'ridan-to'g'ri host'da ishlaydi. Hozircha bosqichma-bosqich
  qo'shilmoqda — pastdagi holatga qarang.

Kirish huquqi ikki xil: **User** (faqat load balancing serverlarga) va **Admin**
(load balancing + boshqaruv serverining o'ziga). Huquqsiz qurilma hech qayerga
kira olmaydi (default-deny).

## Holat (nima tayyor, nima yo'q)

| Bosqich | Nima | Holat |
|---|---|---|
| Phase 0 | DB sxema, auth (JWT+bcrypt+TOTP), RBAC (super_admin/admin), core API, admin panel (7 bo'lim) | ✅ Tayyor |
| Phase 1 | `netdiscd` — LAN qurilmalarini avtomatik topish (ARP/DHCP/SNMP/hostapd) | ✅ Tayyor |
| Phase 2 | `fwctl` — nftables ACL sinxronizatsiyasi, DDoS himoyasi | ✅ Tayyor |
| Phase 3 | Gateway/DHCP/NAT to'liq integratsiyasi | ✅ Tayyor |
| Phase 4 | `lbd` — L4 load balancer, VIP-per-guruh | ✅ Tayyor |
| Phase 5 | `backendagentd` — backend serverlar metrikasi (push-agent) | ✅ Tayyor |
| Phase 6 | `capd` — on-demand pcap yozib olish | ⏳ Keyingi |
| Phase 7 | WireGuard (masofaviy Wireless LAN) | ⏳ |

Hozirgi holatda: `netdiscd` LAN qurilmalarini ARP jadvali, dnsmasq lease
fayli, SNMP (boshqariladigan switch) va hostapd (lokal WiFi) orqali avtomatik
topib, `devices`/`switch_ports` jadvallariga yozadi (barcha 4 manba real
sinaldi). LAN sahifasida admin har bir topilgan qurilmaga nickname va
User/Admin huquq bera oladi, va bu huquq endi **`fwctl` orqali nftables
darajasida haqiqatan kuchga kiradi**: default-deny, User → faqat load
balancing (forward), Admin → load balancing + "o'rtadagi server" (input),
DDoS baseline meter'lari bilan. `fwctl` endi shu bilan bir qatorda **haqiqiy
gateway** ham bo'la oladi: `FWCTL_WAN_INTERFACE` sozlansa, LAN qurilmalari
uchun NAT (masquerade) qo'shiladi va har bir ruxsat qoidasiga WAN
interfeysidan kelgan (potentsial soxta MAC) trafikni bloklovchi himoya
qo'shiladi. DHCP endi haqiqiy `dnsmasq` orqali beriladi (`deploy/dnsmasq/`).
Butun zanjir (DHCP → Postgres → aclsync → fwctl → nftables NAT → real
"internet" trafik) `ip netns` orqali qurilgan 3-tugunli izolyatsiyalangan
topologiyada haqiqiy paketlar bilan sinaldi — internet-tomon serverning o'z
logi LAN mijozining haqiqiy IP'si emas, gateway'ning WAN IP'sini ko'rsatishi
orqali NAT tasdiqlandi.

Endi `lan_forward` orqali ruxsat berilgan trafik haqiqiy load-balancing
serverlarga ham boradi: `lbd` har bir server guruhi uchun VIP manzilini
(`/32`, LAN interfeysida) ko'taradi, real TCP ulanishlarni `round_robin`
yoki `least_conn` bo'yicha tanlangan backend'ga proksi qiladi, va har 3
soniyada TCP-connect health check qiladi (istalgan turdagi server uchun
ishlaydi — HTTP shart emas). `netdiscd`/`fwctl` kabi `lbd` ham Postgres'ga
bevosita ulanmaydi — `internal/lbsync` `server_groups`/`backend_servers`ni
o'qib unga push qiladi va sog'liq natijalarini orqaga yozadi. 4-tugunli
`ip netns` topologiyasida haqiqiy backend'lar (python http.server) bilan
sinaldi: round-robin almashinuvi, backend o'chganda avtomatik chetlashtirish
(health check + jonli trafik ikkalasi ham tekshirildi), tiklanganda
qaytadan ishga qo'shilishi, `least_conn`ning haqiqiy faol-ulanish soniga
qarab tanlashi, guruhni o'chirish/DB orqali faolsizlantirish VIP'ni to'g'ri
bo'shatishi, va butun boshqaruv zanjiri (real Postgres → API → lbd → orqaga
Postgres) uchtan-uchgacha tasdiqlandi.

Endi har bir backend serverning haqiqiy host metrikasi (CPU/RAM/disk/tarmoq)
ham ko'rinadi: `backendagentd` — gateway'da emas, **backend serverning
o'zida** ishlaydigan kichik agent — o'z hostini o'lchab, control-plane
API'ga tarmoq orqali push qiladi (`internal/hostmetrics`, Server bo'limi
ishlatgan o'sha gopsutil sampler'ining qayta ishlatilgan versiyasi). Har bir
backend qo'shilganda avtomatik tasodifiy token yaratiladi (Serverlar
sahifasida ko'rinadi, kerak bo'lsa yangilanadi) — agent shu token bilan
`/api/agent/metrics`ga autentifikatsiya qiladi, JWT emas (bu chaqiruvchi
tizimga kirgan admin emas, tarmoqdagi boshqa mashina). Real Postgres + real
`cmd/api` + real `cmd/backendagentd` bilan uchtan-uchgacha sinaldi: haqiqiy
metrikalar bazaga tushdi, noto'g'ri/eskirgan token 401 bilan rad etildi,
tokenni yangilash eskisini darhol ishlamay qo'ydi, va Serverlar sahifasida
(haqiqiy brauzerda, Playwright orqali) token nusxalash va jonli
CPU/RAM/Disk grafigi konsolda xatosiz ishlashi tasdiqlandi.

## Loyiha tuzilmasi

```
cmd/api/              REST API entrypoint
cmd/netdiscd/          LAN qurilma topish daemoni (Phase 1) entrypoint
cmd/fwctl/             nftables ACL enforcement daemoni (Phase 2) entrypoint
cmd/lbd/               L4 load balancer daemoni (Phase 4) entrypoint
cmd/backendagentd/     Backend server metrikasi push-agenti (Phase 5) entrypoint —
                       gateway'da emas, har bir backend serverda ishlaydi
internal/
  config/              Muhit o'zgaruvchilarini o'qish
  db/                  Postgres ulanish + o'rnatilgan (embed) migratsiyalar
  models/              Domen tiplari
  auth/                JWT, bcrypt, TOTP
  httpapi/             HTTP handlerlar, middleware, router
  hostmetrics/          CPU/RAM/Disk/Net sampler (gopsutil) — metrics VA backendagentd
                        ikkalasi ham shu yerdan foydalanadi
  metrics/             Gateway'ning o'z host metrikasini yig'uvchi (hostmetrics ustida)
  netdisc/              netdiscd'ning kollektorlari (ARP/dnsmasq/SNMP/hostapd) + Unix-socket server
  discovery/            API tomonida netdiscd snapshot'ini Postgres'ga sinxronlash
  firewall/             fwctl'ning nftables ruleset generator/apply/manager/socket serveri
  aclsync/               API tomonida access_grants'ni Postgres'dan o'qib fwctl'ga push qiluvchi
  lb/                  lbd'ning VIP/proksi/health-check/manager + Unix-socket serveri
  lbsync/               API tomonida server_groups/backend_servers'ni Postgres'dan o'qib lbd'ga push qiluvchi va sog'liqni orqaga yozuvchi
web/                   React + TypeScript + Vite admin paneli
deploy/
  docker/              Control-plane uchun Dockerfile'lar va docker-compose.yml
  systemd/             Data-plane daemonlar uchun systemd unit fayllari (bosqichma-bosqich)
  dnsmasq/             Haqiqiy DHCP server uchun tayyor dnsmasq konfiguratsiya namunasi
docs/deploy.md          To'liq deploy qo'llanmasi
```

## Tezkor ishga tushirish (development)

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

`http://localhost:5173` ga kirib, yuqoridagi bootstrap login/parol bilan tizimga kiring.

Production deploy (Docker Compose + systemd) uchun: **[docs/deploy.md](docs/deploy.md)**.
