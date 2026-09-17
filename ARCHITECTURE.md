# ARCHITECTURE.md — LAN Access & Load-Balancing Control Platform

> Bu hujjat loyihaning **yangi** (Zabbix'ga o'xshash, lekin soddalashtirilgan) arxitekturasini belgilaydi. Eski `p1-3server` (Docker + nginx demo) loyihaning **g'oyasi** — ya'ni yuqori ishonchli, yukni taqsimlovchi veb-xizmat — saqlanib qoladi, lekin **struktura to'liq qayta quriladi**: endi bu shunchaki load balancer emas, balki butun LAN tarmog'ini boshqaruvchi, kirish huquqlarini nazorat qiluvchi va trafikni monitoring qiluvchi to'liq platforma.

---

## 0. Qaror jurnali (Decision Log)

Loyihani boshlashdan oldin so'ralgan savollar va foydalanuvchi javoblari — keyingi barcha dizayn qarorlari shularga asoslanadi:

| # | Savol | Qaror |
|---|-------|-------|
| 1 | Muhit | **Real production server** (jismoniy/virtual, ko'p NIC) |
| 2 | "O'rtadagi server" nima | **Shu boshqaruv/monitoring serverining o'zi** — adminlar to'g'ridan-to'g'ri kira oladi, userlar kira olmaydi |
| 3 | Tech stack | Ochiq qoldirilgan → **Claude tanlaydi** (professional darajada) |
| 4 | DHCP/Firewall | **Tayyor vositalar** (dnsmasq + nftables) ustida quramiz |
| 5 | Tarmoq uskunasi | **Boshqariladigan (managed) switch** + serverda **WiFi karta** bor |
| 6 | Docker qamrovi | **Tarmoq xizmatlari (DHCP/firewall/LB/capture) host'da**, **veb-panel + DB Docker'da** |
| 7 | Autentifikatsiya | **Login+parol (+ ixtiyoriy 2FA) VA IP/MAC cheklovi** — ikki qatlamli |
| 8 | Trafik yozib olish | **On-demand** (admin "Start" bosganda) + **avtomatik rotatsiya** (hajm/vaqt bo'yicha) + **umumiy disk kvotasi** |
| 9 | Guruh IP manzillari | **Har bir server guruhi uchun alohida VIP (IP alias)** — yetarli bo'sh IP bor |
| 10 | Miqyos | **O'rta** — 50–300 qurilma |
| 11 | VPN (masofaviy Wireless LAN) | **WireGuard** |
| 12 | UI/UX | Zamonaviy, minimalist, real-vaqt grafiklar — **Claude to'liq dizaynni tanlaydi** |
| 13 | OS | **Ubuntu Server LTS** |
| 14 | Gateway roli | **Ha** — bu server LAN'ning **asosiy shlyuzi (gateway+NAT)**, barcha trafik (internet + ichki) shu orqali o'tadi |
| 15 | Admin ierarxiyasi | **2 daraja**: Super-admin va oddiy Admin |
| 16 | DDoS chegaralari | **Aqlli standart qiymatlar** (kodda hujjatlashtirilgan holda) |

**Muhim cheklov:** Bu Claude Code sessiyasi bulutli, izolyatsiyalangan konteynerda ishlaydi — real LAN, boshqariladigan switch, WiFi karta yoki root darajasidagi tarmoq huquqlariga ega emas. Shuning uchun bu yerda **barcha kod yoziladi va unit-test qilinadi**, lekin DHCP/firewall/WireGuard/packet-capture kabi qismlarning **to'liq end-to-end sinovi faqat real Ubuntu serverda** o'tkazilishi kerak bo'ladi. Kodni shunday yozamanki, u real serverga deploy qilinganda ishlashi kerak, lekin bu yerdan "ishladi" deb kafolat berolmayman — bu haqda har bir bosqichda ochiq aytib boraman.

---

## 1. Yuqori darajadagi arxitektura

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
   [LAN qurilmalari — kabelli]              [Uzoqdagi Wireless LAN,
   [LAN qurilmalari — WiFi]                  internet orqali tunnel]
```

Serverning ichki qatlamlari (bitta Ubuntu host ichida):

```
┌─────────────────────────────────────────────────────────────────┐
│                         UBUNTU SERVER (Host)                     │
│                                                                    │
│  DATA-PLANE (root/CAP_NET_ADMIN, systemd, host tarmog'ida) :      │
│  ┌───────────┐ ┌──────────┐ ┌─────────┐ ┌────────┐ ┌───────────┐ │
│  │ dnsmasq   │ │ nftables │ │ netdiscd│ │  lbd   │ │   capd    │ │
│  │ (DHCP+DNS)│ │(firewall)│ │(topilma)│ │(L4 LB) │ │(pcap)     │ │
│  └───────────┘ └──────────┘ └─────────┘ └────────┘ └───────────┘ │
│         ▲             ▲           ▲          ▲            ▲       │
│         └─────────────┴───── gRPC/Unix-socket ────────────┘       │
│                              │                                    │
│  CONTROL-PLANE (Docker, imtiyozsiz):                              │
│  ┌────────────────────┐   ┌──────────────────┐                   │
│  │   API server (Go)  │◄──┤  PostgreSQL +     │                   │
│  │  Auth, RBAC, REST/  │   │  TimescaleDB      │                   │
│  │  WebSocket          │   └──────────────────┘                   │
│  └─────────┬───────────┘                                          │
│            │ REST + WebSocket (real-time)                         │
│  ┌─────────▼───────────┐                                          │
│  │  Admin Web Panel     │  (React + TS, faqat adminlarga ochiq)   │
│  │  (Docker, nginx)     │                                          │
│  └──────────────────────┘                                         │
└─────────────────────────────────────────────────────────────────┘
```

**Nega shunday bo'lindi:** Privilegiyali ishlarni (paket filtrlash, DHCP, xom soket, tcpdump) faqat kichik, tekshirilishi oson bo'lgan Go daemon'lar bajaradi (har biri bitta vazifaga mas'ul, systemd orqali host'da). API server esa hech qanday maxsus huquqqa ega emas — u faqat Postgres bilan va daemon'larning mahalliy (localhost-only) boshqaruv socketlari bilan gaplashadi. Bu **xavfsizlik** (hujum yuzasi kichik) va **tozalik** (kod bir-biriga bog'lanmagan) uchun professional andoza.

---

## 2. Foydalanuvchi/qurilma oqimi (kim qayerga kira oladi)

| Qurilma turi | Load balancing serverlarga (VIP) | "O'rtadagi" boshqaruv serveriga | Internetga |
|---|---|---|---|
| **User** guruhidagi IP/MAC | ✅ Ha | ❌ Yo'q | ✅ Ha (agar administrator ruxsat bersa — kerak bo'lsa cheklanishi ham mumkin) |
| **Admin** guruhidagi IP/MAC | ✅ Ha | ✅ Ha | ✅ Ha |
| Ro'yxatga olinmagan (na user, na admin) | ❌ Yo'q | ❌ Yo'q | ❌ Yo'q |

Bu qoida **nftables**da ikkita named set orqali amalga oshiriladi: `set allowed_user { type ether_addr; }` va `set allowed_admin { type ether_addr; }`. Default policy — **DROP**. `fwctl` daemoni bu setlarni ma'lumotlar bazasidagi `devices`/`access_grants` jadvaliga mos ravishda doimiy sinxronlab turadi (real-vaqtga yaqin, ~1-2 soniya reconciliation loop).

---

## 3. Texnologiya steki (yakuniy tanlov)

| Qatlam | Texnologiya | Asoslanish |
|---|---|---|
| Data-plane daemonlar | **Go** | Yagona binary, past xotira, systemd bilan ajoyib integratsiya, root-level tarmoq ishlari uchun standart tanlov (Docker, Kubernetes, Consul kabi loyihalar ham shunday) |
| Control-plane API | **Go** (chi/Fiber router) | Daemon'lar bilan bitta tilda — kod bazasi bir xil, umumiy struct/proto ta'riflarini bo'lishish oson |
| Ma'lumotlar bazasi | **PostgreSQL 16 + TimescaleDB** | Relyatsion ma'lumotlar (userlar, qurilmalar, guruhlar) + vaqt-qatori metrikalar (CPU/RAM/trafik) bitta DB'da, professional va operatsion jihatdan sodda |
| DHCP | **dnsmasq** | Yengil, ishonchli, lease-fayli va hook-skriptlari orqali oson integratsiya qilinadi |
| Firewall | **nftables** | Zamonaviy Linux standarti (iptables o'rnini bosgan), named sets orqali dinamik ACL uchun ideal |
| L4 Load Balancer | Custom **Go** (`lbd`) — TCP proxy, VIP-per-group | Talab "istalgan turdagi TCP xizmatini" (web, DB, va h.k.) taqsimlash — bu HAProxy/nginx stream'ga o'xshash, lekin bizning admin panelimizga chuqur integratsiya qilingan bo'ladi |
| Paket ushlash | **libpcap** asosidagi `capd` (tcpdump'ni wrap qiladi) | .pcap format to'g'ridan-to'g'ri Wireshark bilan ochiladi |
| VPN | **WireGuard** (`wg-quick`, kernel modul) | Eng tez va sodda site-to-site tunnel |
| Frontend | **React + TypeScript + Vite + TailwindCSS** | Zamonaviy, tez, minimalist dizayn uchun eng mos; komponentlar orqali "kvadrat bo'limlar" tuzilmasini oson qurish mumkin |
| Real-vaqt grafiklar | **ECharts** (yoki Recharts) + WebSocket | Chiroyli, interaktiv, real-vaqt yangilanadigan grafiklar |
| Auth | JWT + bcrypt + TOTP (2FA, ixtiyoriy) | Standart, xavfsiz, ko'p kutubxonalar bilan qo'llab-quvvatlanadi |
| Deploy | systemd (daemonlar) + Docker Compose (Postgres, API, Frontend) | Foydalanuvchi tanlagan gibrid model |

---

## 4. Ma'lumotlar bazasi sxemasi (asosiy jadvallar)

```
admins(id, username, password_hash, totp_secret, role[super_admin|admin],
       created_by_admin_id, created_at, last_login_at)

devices(id, mac_address UNIQUE, ip_address, hostname, nickname,
        conn_type[wired|wireless], switch_port_id NULL, ssid NULL,
        first_seen_at, last_seen_at, is_online)

access_grants(id, device_id, role[user|admin], granted_by_admin_id, granted_at)
        -- device_id uchun grant bo'lmasa = bloklangan (default deny)

switch_ports(id, switch_name, port_number, label, vlan, link_status, last_change_at)

server_groups(id, nickname, color_hex, vip_address, protocol[tcp],
              algorithm[round_robin|least_conn], is_active)

backend_servers(id, group_id, ip, port, weight, health_status,
                 last_check_at, response_time_ms)

vpn_peers(id, name, public_key, allowed_subnet, endpoint, is_active,
          last_handshake_at)

lan_networks(id, name, type[wired|wireless_local|wireless_remote_vpn],
             vpn_peer_id NULL, is_active, last_status_check_at, is_reachable)

traffic_captures(id, device_id, started_by_admin_id, file_path,
                  started_at, stopped_at, size_bytes,
                  status[recording|rotated|downloaded], rotation_reason)

audit_logs(id, actor_admin_id, action, target_type, target_id, details_json, created_at)

-- TimescaleDB hypertables (vaqt-qatori):
system_metrics(time, cpu_percent, mem_percent, disk_read_bps, disk_write_bps,
                net_in_bps, net_out_bps, disk_percent)
device_traffic_stats(time, device_id, backend_group_id, bytes_in, bytes_out)
```

---

## 5. Admin panel — sahifalar (foydalanuvchi talabiga mos)

Bosh sahifa — 6 ta kvadrat bo'lim (kartochka), boshqa hech narsa yo'q:

1. **Server** — bu (LB/gateway) serverning o'zi haqida: CPU, RAM, disk I/O, tarmoq tezligi — real-vaqt grafiklar (ECharts).
2. **Userlar** — jadval: Nickname | Device name | IP | MAC. Bosilganda: shu userning qaysi backend guruhga qancha trafik yuborayotgani + **"Trafikni yozib olish" (pcap) tugmasi**.
3. **Adminlar** — Userlar bilan bir xil jadval/ko'rinish, lekin pcap tugmasisiz. Super-admin qo'shimcha ravishda boshqa adminlarni qo'sha/o'chira oladi.
4. **Serverlar** — Load balancing guruhlari: nickname + rang + VIP + ichidagi backend serverlar ro'yxati (ip:port, weight, health).
5. **LAN** — 2 ta jadval: **Portlar** (kabelli, switch porti asosida) va **Wireless** (WiFi mijozlari + masofaviy VPN LAN'lar). Har birini bosib: nomini o'zgartirish yoki huquq berish (User/Admin/Yo'q). Har bir LAN (jumladan VPN orqali ulangan masofaviy LAN) uchun Active/Deactive tugmasi + real-vaqt status (bor/yo'q, past kechikish bilan tekshiriladi).
6. **Firewall** — hozirgi holat: nechta qurilma bloklangan/ruxsat berilgan, DDoS himoyasi statistikasi (real-vaqt).
7. **Logs** — Barcha `traffic_captures` ro'yxati: kim, qaysi user, fayl qancha vaqtdan beri yozilyapti, **Download** tugmasi (bosilganda: joriy faylni yakunlaydi va yuklab beradi, yozishni **yangi fayl bilan davom ettiradi**).

---

## 6. Amalga oshirish bosqichlari (Roadmap)

| Bosqich | Mazmuni |
|---|---|
| **Phase 0** | Repo tozalash (eski Docker/nginx demo olib tashlanadi), yangi monorepo skeleton (`/cmd/api`, `/cmd/netdiscd`, `/cmd/fwctl`, `/cmd/lbd`, `/cmd/capd`, `/web`), Postgres migratsiyalar, auth (login/parol/2FA/JWT), bo'sh admin panel (6 kvadrat, navigatsiya) |
| **Phase 1** | `netdiscd`: ARP skanerlash + dnsmasq lease kuzatuvi + SNMP port polling + hostapd stansiya ro'yxati → `devices` jadvali. LAN sahifasi: real ma'lumot, nickname/huquq berish |
| **Phase 2** | `fwctl`: nftables setlarini DB bilan sinxronlash, default-deny, DDoS baseline qoidalar. Firewall sahifasi |
| **Phase 3** | Gateway/NAT/DHCP to'liq integratsiyasi, "o'rtadagi server"ga faqat admin guruhidan kirish qoidasi |
| **Phase 4** | `lbd`: VIP-per-group L4 load balancer, Serverlar sahifasi (guruh CRUD, rang/nickname) |
| **Phase 5** | Server metrikalari (CPU/RAM/Disk/Net) yig'uvchi + Server sahifasidagi grafiklar |
| **Phase 6** | `capd`: on-demand pcap yozib olish, rotatsiya, kvota, Logs sahifasi |
| **Phase 7** | WireGuard site-to-site (masofaviy Wireless LAN), LAN sahifasida real-vaqt reachability |
| **Phase 8** | Super-admin/Admin RBAC to'liq qo'llanilishi, audit log, xavfsizlik audit, yakuniy dizayn siyqallashtirish |

---

## 7. Keyingi qadam

Ushbu hujjat tasdiqlansa, **Phase 0** dan boshlayman: eski Docker/nginx fayllarini olib tashlab, yangi Go monorepo + Postgres + React skeletonni qurib, PR #1 ustiga push qilaman. Agar biror joyda tuzatish yoki qo'shimcha talab bo'lsa — shu yerda ayting, boshlashdan oldin arxitekturani moslashtiraman.
