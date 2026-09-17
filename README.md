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
| Phase 1 | `netdiscd` — LAN qurilmalarini avtomatik topish (ARP/DHCP/SNMP/hostapd) | ⏳ Keyingi |
| Phase 2 | `fwctl` — nftables ACL sinxronizatsiyasi, DDoS himoyasi | ⏳ |
| Phase 3 | Gateway/DHCP/NAT to'liq integratsiyasi | ⏳ |
| Phase 4 | `lbd` — L4 load balancer, VIP-per-guruh | ⏳ |
| Phase 5 | Backend serverlar metrikasi | ⏳ |
| Phase 6 | `capd` — on-demand pcap yozib olish | ⏳ |
| Phase 7 | WireGuard (masofaviy Wireless LAN) | ⏳ |

Hozirgi holatda: devices/admins/server-groups/lan-networks uchun to'liq CRUD API
va admin panel ishlaydi, real tizim metrikalari (CPU/RAM/Disk/tarmoq) yig'ilib
grafik ko'rinishda chiqadi — faqat hali ma'lumotlar qo'lda (yoki keyingi
bosqichlardagi daemonlar orqali) kiritiladi, avtomatik LAN skanerlash yo'q.

## Loyiha tuzilmasi

```
cmd/api/              REST API entrypoint
internal/
  config/              Muhit o'zgaruvchilarini o'qish
  db/                  Postgres ulanish + o'rnatilgan (embed) migratsiyalar
  models/              Domen tiplari
  auth/                JWT, bcrypt, TOTP
  httpapi/             HTTP handlerlar, middleware, router
  metrics/             Host CPU/RAM/Disk/Net metrikalarini yig'uvchi
web/                   React + TypeScript + Vite admin paneli
deploy/
  docker/              Control-plane uchun Dockerfile'lar va docker-compose.yml
  systemd/             Data-plane daemonlar uchun systemd unit fayllari (bosqichma-bosqich)
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
