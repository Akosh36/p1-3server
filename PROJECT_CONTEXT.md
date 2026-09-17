# p1-3server — Yuqori Ishonchlilikka Ega (High-Availability) Veb-server Infratuzilmasi

> Ushbu fayl loyihaning **to'liq konteksti**ni bitta joyga jamlaydi: arxitektura, fayllar tuzilmasi, konfiguratsiyalar, skriptlar, boshqaruv paneli (dashboard) va ishga tushirish/testlash bo'yicha ko'rsatmalar. Fayl repo ichidagi barcha `.md`/`.txt` hujjatlarni, konfiguratsiya fayllarini va skriptlarni o'rganib chiqib, ularning mazmunini birlashtirish orqali tayyorlangan.

---

## 1. Loyiha haqida umumiy ma'lumot

Bu — **lokal tarmoqda (LAN) ishlaydigan, xatolarga bardoshli (fault-tolerant), yukni taqsimlovchi (load-balanced)** veb-xizmat arxitekturasi. Butun tizim Docker konteynerlarida (nginx:alpine image asosida) ishlaydi va backend serverlardan biri (yoki bir nechtasi) ishdan chiqsa ham, foydalanuvchi uchun uzilishsiz xizmat ko'rsatishni ta'minlaydi (avtomatik failover).

Loyiha 2 ta asosiy qismdan iborat:
1. **Asosiy infratuzilma** — Nginx load balancer + bir nechta Nginx backend web-server konteynerlari (repo ildizida).
2. **Monitoring/boshqaruv paneli** (`view/` papkasi) — Flask asosidagi veb-interfeys, real vaqtda monitoring va serverlarni boshqarish uchun.

### Asosiy xususiyatlar

- ✅ Silliq failover — 1 yoki 2 ta backend server ishdan chiqsa, trafik avtomatik boshqa serverlarga yo'naltiriladi.
- ✅ Uzilishsiz ishlash — foydalanuvchi server ishdan chiqishini sezmaydi.
- ✅ Yuk taqsimlash — round-robin (standart), least_conn yoki ip_hash usullari bilan.
- ✅ Sog'liqni tekshirish (health check) — Nginx upstream orqali `max_fails`/`fail_timeout` parametrlari bilan.
- ✅ Internetga bog'liq emas — butun tizim lokal Docker tarmog'ida ishlaydi.
- ✅ Failoverni sinash oson — konteynerlarni to'xtatish/o'ldirish orqali.
- ✅ Veb-asosidagi boshqaruv paneli — serverlarni start/stop/restart qilish, yangi server qo'shish, HTML fayl yuklash, load balancer sozlamalarini o'zgartirish, xostlarni monitoring qilish.

### Arxitektura sxemasi

```
┌─────────────────────┐
│   LAN foydalanuvchi │
│  (http://host:80)   │
└──────────┬──────────┘
           │
    ┌──────▼──────────────────┐
    │  Load Balancer (Nginx)  │
    │  - Reverse Proxy        │
    │  - Health Checks        │
    │  - Avtomatik Failover   │
    └──────┬──────┬───────┬───┘
           │      │       │
    ┌──────▼──┐ ┌──┴──┐ ┌─┴──────┐
    │ Web #1  │ │Web  │ │ Web #3 │
    │ Nginx   │ │ #2  │ │ Nginx  │
    │ :8001   │ │Nginx│ │ :8003  │
    │         │ │:8002│ │        │
    └─────────┘ └─────┘ └────────┘
```

Load balancer 80-portda tinglaydi va so'rovlarni backend serverlarga (127.0.0.1:8001, 8002, 8003, ...) proxy qiladi. `docker-compose.yml` faylida esa qo'shimcha ravishda `web_server_4`...`web_server_7` (portlar 8004–8007) ham ta'riflangan — loyiha vaqt o'tishi bilan 3 tadan ko'proq backend serverga kengaytirilgan.

---

## 2. Repozitoriy fayl tuzilmasi

```
p1-3server/
├── README.md                    # To'liq hujjat (300+ qator) — asosiy qo'llanma
├── QUICKSTART.md                 # Tezkor boshlash qo'llanmasi
├── MONITORING_GUIDE.md           # Monitoring dashboard bo'yicha batafsil qo'llanma
├── SETUP_SUMMARY.txt             # O'rnatish yakunlangani haqida qisqa xulosa
├── docker-compose.yml            # Docker Compose orqali barcha konteynerlarni ishga tushirish
├── nginx.conf                    # Load balancer konfiguratsiyasi (upstream + health check)
├── nginx.conf.backup.*           # nginx.conf ning turli sanalardagi zaxira nusxalari (7 ta)
├── nginx_backend.conf            # Umumiy backend server konfiguratsiya namunasi
├── nginx_backend_server.conf     # Backend konteynerlar uchun umumiy default.conf
├── nginx_8000.conf … nginx_8099.conf,
│   nginx_80010.conf, nginx_8027.conf,
│   nginx_8030.conf, nginx_8066.conf     # Har bir port/backend server uchun alohida Nginx konfiglar
├── index.html                    # Barcha backend serverlar tomonidan taqdim etiladigan asosiy HTML sahifa
├── control-scripts/               # Infratuzilmani boshqarish uchun Bash skriptlar
│   ├── start.sh / stop.sh
│   ├── start1.sh, start2.sh, start3.sh
│   ├── stop1.sh, stop2.sh, stop3.sh
│   ├── start_servers.sh / stop_servers.sh / restart_servers.sh
│   ├── add_server.sh
│   ├── create_docker_server.sh
│   ├── configure_load_balancer.sh
│   ├── update_load_balancer.sh
│   ├── upload_html.sh
│   ├── monitor_hosts.sh
│   └── ssh_login.sh
└── view/                          # Flask monitoring & boshqaruv dashboard
    ├── monitor.py                 # Flask backend (asosiy server kodi)
    ├── requirements.txt           # Python bog'liqliklari
    ├── run.sh / setup.sh          # Ishga tushirish/o'rnatish skriptlari
    ├── README.md                  # Dashboard bo'yicha batafsil hujjat
    ├── USAGE_GUIDE.txt            # Dashboard foydalanish bo'yicha vizual qo'llanma
    ├── dashboard.log              # Runtime log fayli
    ├── __pycache__/               # Python bytecode cache (monitor.py, management_server.py)
    └── templates/
        ├── dashboard.html         # Monitoring dashboard sahifasi (Bootstrap 5 + Chart.js uslubida)
        └── management.html        # Serverlarni boshqarish sahifasi
```

> Eslatma: `nginx.conf.backup.*` fayllari `update_load_balancer.sh` skripti tomonidan avtomatik yaratilgan zaxira nusxalar (upstream blokini yangilashdan oldin).

---

## 3. Asosiy infratuzilma komponentlari

### 3.1. `docker-compose.yml`

- 1 ta `load_balancer` konteyneri (nginx:alpine, 80-portni tashqariga ochadi, `nginx.conf` ni ulaydi).
- `web_server_1`...`web_server_7` konteynerlari (nginx:alpine, `network_mode: host`, har biri o'z portida: 8001–8007).
- Har bir backend konteyner `index.html` faylini va o'ziga tegishli nginx konfiguratsiya faylini (`nginx_backend_server.conf` yoki `nginx_800X.conf`) `:ro` (read-only) rejimida bog'laydi.
- Barcha konteynerlar `restart: unless-stopped` siyosatiga ega.
- `load_balancer` — `web_server_1..5` ga bog'liq (`depends_on`).

### 3.2. `nginx.conf` — Load Balancer konfiguratsiyasi

Asosiy qismlari:
- `upstream backend` bloki — hozirda quyidagi serverlarni o'z ichiga oladi:
  - `127.0.0.1:8001`, `127.0.0.1:8002`, `127.0.0.1:8003`, `127.0.0.1:8066` — har biri `max_fails=3 fail_timeout=10s` bilan.
  - `keepalive 32;` — backendlarga ulanishlarni qayta ishlatish uchun.
- `limit_req_zone` — so'rovlarni cheklash (rate limiting) uchun (100 so'rov/sekund, IP bo'yicha).
- `server` bloki (80-port):
  - `location /` — `proxy_pass http://backend;` orqali barcha so'rovlarni upstream'ga proxy qiladi (HTTP/1.1, keep-alive header'lari, timeout'lar, buferlash sozlamalari bilan).
  - `location /health` — sodda health-check endpoint, doim `200 healthy` qaytaradi.
  - `location ~ /\.` — yashirin fayllarga (`.` bilan boshlanuvchi) kirishni taqiqlaydi.

**Health check parametrlari:**

| Parametr | Ma'nosi | Xatti-harakati |
|----------|---------|----------------|
| `max_fails=3` | 3 marta muvaffaqiyatsiz so'rovdan keyin | Server vaqtincha "down" deb belgilanadi |
| `fail_timeout=10s` | Timeout oynasi | 10 soniyadan keyin server qayta sinaladi |
| `keepalive 32` | Ulanishlarni saqlash | Backendlarga ulanishlarni qayta ishlatish (samaradorlik) |

**Failover qanday ishlaydi (bosqichma-bosqich):**
1. Foydalanuvchi so'rov yuboradi → Load balancer qabul qiladi.
2. Nginx birinchi mavjud upstream (`web_server_1`) ga urinadi.
3. So'rov muvaffaqiyatsiz bo'lsa → hisoblagich oshadi (masalan, 1/3).
4. 4-chi muvaffaqiyatsiz urinishda → `web_server_1` "down" deb belgilanadi.
5. Keyingi so'rovlar → faqat `web_server_2` yoki `web_server_3` ga yo'naltiriladi.
6. `fail_timeout` (10s) dan keyin → `web_server_1` yana bir marta sinaladi.
7. Muvaffaqiyatli bo'lsa → server upstream pool'ga qaytadi.

### 3.3. Backend konfiguratsiyalari

- **`nginx_backend.conf`** / **`nginx_backend_server.conf`** — barcha backend konteynerlar uchun umumiy `default.conf` namunasi: statik `index.html` ni `try_files` orqali xizmat qiladi, 404 xatoligini qayta yo'naltiradi.
- **`nginx_800X.conf`** (masalan `nginx_8001.conf`, `nginx_8004.conf` va h.k.) — har bir port uchun alohida to'liq nginx konfiguratsiyasi (ba'zilari to'liq `http {}` blokini o'z ichiga oladi, ba'zilari faqat `server {}` blokini — bu ikki xil formatdagi fayllar mavjudligini bildiradi, chunki turli skriptlar ularni turlicha yaratgan).
- Konfiguratsiyalarning aksariyati `location /health` endpointiga ega — bu load balancer tomonidan server holatini tekshirish uchun ishlatiladi (garchi hozirgi nginx.conf faol health-check uchun faqat passive health check — `max_fails`/`fail_timeout` — ishlatsa ham).

### 3.4. `index.html`

O'zbek tilida yozilgan sodda va chiroyli statik sahifa ("Veb-server ishlamoqda"). U:
- Yuk taqsimlash va qayta tiklanish haqida tushuntirish beradi (oddiy foydalanuvchi uchun).
- "✅ Sayt normal ishlayapti" degan yashil belgi ko'rsatadi.
- JavaScript orqali sahifa yuklangan vaqtni (`timestamp`) `uz-UZ` formatida ko'rsatadi.
- Barcha backend serverlar (`web_server_1..N`) ushbu bitta faylni taqdim qiladi — shuning uchun qaysi serverga tushganingizni bilib bo'lmaydi (bu ataylab load-balancing shaffofligini ko'rsatish uchun).

---

## 4. `control-scripts/` — Boshqaruv skriptlari

Barcha skriptlar loyihani `PROJECT_DIR="/home/akobir/Documents/Projects/DProjects/p1-3server"` yo'liga qattiq bog'langan holda ishlatadi (ko'pchilik skriptlarda `cd "$PROJECT_DIR"` bor). Bu — skriptlar dastlab muallifning shaxsiy kompyuterida ishlab chiqilganini bildiradi va boshqa muhitda ishlatishda bu yo'lni moslashtirish kerak bo'lishi mumkin.

| Skript | Vazifasi | Ishlatilishi |
|--------|----------|---------------|
| `start.sh` | `docker run` orqali load_balancer + web_server_1/2/3 ni to'g'ridan-to'g'ri (docker-compose'siz) ishga tushiradi | `./start.sh` |
| `stop.sh` | Barcha asosiy konteynerlarni to'xtatib, o'chiradi | `./stop.sh` |
| `start1.sh` / `start2.sh` / `start3.sh` | Faqat mos web_server_N konteynerini `docker start` qiladi | `./start1.sh` |
| `stop1.sh` / `stop2.sh` / `stop3.sh` | Faqat mos web_server_N konteynerini `docker stop` qiladi | `./stop1.sh` |
| `start_servers.sh` | Server(lar)ni yaratadi/ishga tushiradi; agar nginx konfiguratsiya fayli mavjud bo'lmasa, uni avtomatik generatsiya qiladi | `./start_servers.sh [1\|2\|3\|...\|all\|lb]` |
| `stop_servers.sh` | Server(lar)ni to'xtatadi va konteynerni o'chiradi (`docker rm`) | `./stop_servers.sh [N\|all\|lb]` |
| `restart_servers.sh` | `stop_servers.sh` + `start_servers.sh` ketma-ketligini chaqiradi | `./restart_servers.sh [N\|all\|lb]` |
| `add_server.sh` | Yangi backend server uchun: nginx konfiguratsiya faylini yaratadi, `docker-compose.yml` va `nginx.conf` (upstream) ga avtomatik `sed` orqali yangi yozuv qo'shadi | `./add_server.sh <server_num> <port>` |
| `create_docker_server.sh` | `add_server.sh` ga o'xshash, lekin konteynerni bevosita `docker run` bilan ishga ham tushiradi va load balancerga max_fails/fail_timeout bilan qo'shadi | `./create_docker_server.sh <server_num> <port>` |
| `configure_load_balancer.sh` | `nginx.conf` dagi upstream blokini boshqaradi: `list`, `add-server <ip> <port>`, `remove-server <ip> <port>`, `set-method <round_robin\|least_conn\|ip_hash\|weight>`. Har bir amaldan so'ng load balancerni qayta ishga tushiradi | `./configure_load_balancer.sh <action> [params]` |
| `update_load_balancer.sh` | Docker orqali ishlab turgan barcha `web_server_*` konteynerlarini avtomatik aniqlab, `nginx.conf` upstream blokini ularning portlari bilan to'liq qayta yozadi (avval zaxira nusxa oladi) | `./update_load_balancer.sh` |
| `upload_html.sh` | Berilgan `.html` faylni bitta serverga (`docker cp` orqali) yoki barcha serverlarga (lokal `index.html` ni almashtirish orqali) yuklaydi | `./upload_html.sh <file.html> [N\|all]` |
| `monitor_hosts.sh` | Berilgan xost (IP) uchun ping, ssh-check, portlar, tarmoq statistikasi, jarayonlar, tizim ma'lumotlarini tekshiradi | `./monitor_hosts.sh <ip> [ping\|ssh-check\|services\|network\|ports\|processes\|system\|all]` |
| `ssh_login.sh` | Berilgan xostga ulanish mumkinligini (ping + SSH port) tekshirib, so'ng interaktiv SSH sessiya ochadi | `./ssh_login.sh <ip> [user] [port]` |

**Muhim izohlar:**
- `start_servers.sh` va `add_server.sh`/`create_docker_server.sh` bir-biriga o'xshash ammo bir-biridan farqli generatsiya mantiqiga ega (bittasi to'liq `http{}` blokli konfiguratsiya yaratadi, ikkinchisi faqat `server{}` blokli — nginx image ichidagi joylashuviga qarab: `/etc/nginx/nginx.conf` yoki `/etc/nginx/conf.d/default.conf`).
- `configure_load_balancer.sh` va `add_server.sh` skriptlarida `sed` orqali matn almashtirish ishlatiladi — bu qo'lda tahrirlashni avtomatlashtiradi, lekin nozik joylarda (masalan, allaqachon mavjud qatorlarni tekshirish) ehtiyotkorlik talab qiladi.

---

## 5. `view/` — Flask Monitoring & Boshqaruv Dashboard

Bu — asosiy infratuzilmadan **mustaqil** ishlaydigan, uni o'zgartirmaydigan (faqat o'qish/monitoring va control-scripts orqali boshqarish) alohida Flask ilovasi.

### 5.1. Texnologiyalar (`requirements.txt`)

```
Flask==3.0.0
docker==7.1.0
requests==2.32.0
psutil==6.0.0
Werkzeug==3.0.0
```

### 5.2. `monitor.py` — Flask backend

**Konfiguratsiya:**
- `PROJECT_DIR = "/home/akobir/Documents/Projects/DProjects/p1-3server"` — qattiq kodlangan yo'l (control-scripts skriptlariga o'xshab).
- `UPLOAD_FOLDER` — yuklangan HTML fayllar vaqtincha saqlanadigan papka.
- Faqat `.html` fayllarga ruxsat beriladi (`ALLOWED_EXTENSIONS`).

**Docker bilan ishlash strategiyasi:**
- Avval Python `docker` kutubxonasi orqali ulanishga harakat qiladi (`docker.from_env()`, `client.ping()`).
- Agar bu ishlamasa, `subprocess` orqali `docker` CLI buyruqlariga (`docker ps`, `docker stop` va h.k.) o'tadi (fallback mexanizmi).

**Fon rejimidagi monitoring (`update_monitoring_data`):**
- Alohida `daemon` thread'da har 3 soniyada quyidagilarni yangilaydi:
  - `containers` — barcha `load_balancer`/`web_server_*` konteynerlar ro'yxati va holati.
  - `host_info` — CPU, xotira, disk foydalanishi, hostname, lokal IP (`psutil` + `socket`).
  - `servers_status` — har bir backend serverning HTTP orqali sog'ligini tekshirish (status, javob vaqti, status kodi).
  - `load_balancer` — load balancerning (`http://127.0.0.1`) holatini tekshirish.
- Ma'lumotlar `threading.Lock()` bilan himoyalangan umumiy `monitoring_data` lug'atida saqlanadi (thread-safe).

**Asosiy marshrutlar (routes):**

| Marshrut | Metod | Vazifasi |
|----------|-------|----------|
| `/` , `/view/dashboard` | GET | Asosiy monitoring dashboard sahifasini render qiladi |
| `/view/management` | GET | Serverlarni boshqarish sahifasi (skriptlar, konteynerlar, target ro'yxati bilan) |
| `/scripts/run` | POST | `control-scripts/` dagi ro'yxatdagi skriptni xavfsiz tarzda (`shlex.split` bilan) ishga tushiradi |
| `/api/status` | GET | To'liq monitoring ma'lumotlarini JSON qaytaradi |
| `/api/containers` | GET | Faqat konteynerlar ma'lumotini JSON qaytaradi |
| `/api/servers` | GET | Load balancer + backend serverlar sog'ligini JSON qaytaradi |
| `/api/host` | GET | Xost tizim ma'lumotini JSON qaytaradi |
| `/api/system-info` | GET | Batafsil tizim ma'lumoti (CPU, xotira, disk, uptime, jarayonlar soni, OS) |
| `/servers/start` | POST | `start_servers.sh` ni chaqiradi |
| `/servers/stop` | POST | `docker stop` (to'g'ridan-to'g'ri, konteynerni o'chirmasdan) chaqiradi |
| `/servers/restart` | POST | `restart_servers.sh` ni chaqiradi |
| `/servers/add` | POST | `add_server.sh` ni chaqiradi |
| `/servers/remove` | POST | Serverni load balancerdan olib tashlaydi va konteynerni to'xtatadi |
| `/servers/control` | POST | Umumiy start/stop/restart amali (target + action parametrlari bilan) |
| `/servers/create-docker` | POST | `create_docker_server.sh` ni chaqiradi (JSON javob qaytaradi — AJAX uchun) |
| `/upload` | POST | HTML faylni yuklaydi (`secure_filename` bilan xavfsizlashtirilgan), so'ng `upload_html.sh` ga uzatadi |
| `/load-balancer/configure` | POST | `configure_load_balancer.sh` orqali add-server/remove-server/set-method amallarini bajaradi |
| `/load-balancer/update` | POST | `update_load_balancer.sh` ni chaqirib, barcha ishlab turgan serverlarni load balancerga qo'shadi |
| `/monitoring/hosts` | POST | `monitor_hosts.sh` ni chaqiradi, natijani JSON qaytaradi |
| `/ssh/login` | POST | SSH ulanish buyrug'ini tayyorlaydi (JSON qaytaradi, real ulanish veb orqali amalga oshirilmaydi) |

**Xavfsizlik jihatlari (kod ichida ko'zga tashlanadigan narsalar):**
- `/scripts/run` — faqat `list_control_scripts()` orqali ro'yxatga olingan (`control-scripts/` papkasidagi `.sh` bilan tugaydigan) skriptlarni ishga tushirishga ruxsat beradi — bu ixtiyoriy buyruq bajarilishining (arbitrary command execution) oldini olishga urinish.
- `secure_filename()` — fayl yuklashda path traversal'dan himoya qiladi.
- `app.secret_key` — kodda ochiq matn sifatida qattiq yozilgan (`'ha-web-server-monitoring-key-2024'`) — bu **production uchun xavfli** amaliyot, chunki session cookie'larni soxtalashtirish (forge) imkonini beradi. Lokal/demo muhit uchun mo'ljallangan bo'lsa ham, e'tiborga olinishi kerak.
- `app.run(host='0.0.0.0', port=5000, debug=False)` — barcha tarmoq interfeyslariga ochiq (LAN'dan kirish uchun ataylab shunday qilingan).

### 5.3. `templates/dashboard.html` va `templates/management.html`

- Ikkalasi ham **Bootstrap 5.1.3** (CDN orqali) va **Font Awesome 6** ishlatadi, zamonaviy gradient fon va "AutoPilot Dashboard" / "AutoPilot Management" nomlari bilan.
- `dashboard.html` (329 qator) — real vaqtli monitoring: host ma'lumotlari, load balancer holati, backend serverlar sog'lig'i, konteynerlar holati, Chart.js asosidagi grafikalar (javob vaqti chizig'i, tizim salomatligi donut diagrammasi).
- `management.html` (285 qator) — server boshqaruvi: skriptlarni ishga tushirish formalar, target tanlash (server raqami/`all`/`lb`), fayl yuklash formasi, load balancer konfiguratsiyasi formalar.
- Ma'lumotlar `/api/*` endpointlaridan JavaScript orqali muntazam so'ralib, sahifa har 3 soniyada yangilanadi (backenddagi yangilanish davri bilan mos).

### 5.4. Ishga tushirish skriptlari

- **`setup.sh`** — Python3/pip3 borligini tekshiradi, `venv` virtual muhitini yaratadi va `requirements.txt` dan bog'liqliklarni o'rnatadi.
- **`run.sh`** — `venv` mavjud bo'lsa uni faollashtiradi (aks holda avtomatik yaratadi va o'rnatadi), so'ng `python3 monitor.py` ni ishga tushiradi. Ishga tushganda `http://localhost:5000` manzilini chop etadi.

---

## 6. Hujjatlar (mavjud `.md`/`.txt` fayllar) qisqacha xulosasi

### `README.md` (asosiy hujjat, 540+ qator)
- Loyiha umumiy tavsifi, arxitektura, fayllar jadvali.
- `docker-compose up -d` orqali ishga tushirish bosqichlari.
- 4 ta failover test stsenariysi: bitta serverni to'xtatish, ikkita serverni to'xtatish, konteynerni "kill" qilish (crash simulyatsiyasi), serverni qayta ishga tushirish.
- Health check konfiguratsiyasi tushuntirilishi (`max_fails`, `fail_timeout`, `keepalive`).
- Monitoring/debugging buyruqlari (loglarni ko'rish, konteyner holatini tekshirish, backendni to'g'ridan-to'g'ri sinash).
- Kengaytirilgan sozlash: load balancing algoritmini o'zgartirish (round-robin/least_conn/ip_hash), health check parametrlarini sozlash, SSL/TLS qo'shish bo'yicha eslatma.
- To'xtatish/tozalash buyruqlari (`docker-compose stop/down`, image prune).
- **Veb-asosidagi boshqaruv interfeysi** bo'limi — `view/` dashboardining barcha imkoniyatlari va control-scripts skriptlari jadvali bilan.
- Nosozliklarni bartaraf etish (troubleshooting) bo'limi — connection refused, failover ishlamasligi, konteyner crash, boshqa mashinadan kirish muammolari.
- Performance eslatmalari va arxitektura asoslanishi (nima uchun bu dizayn yuqori ishonchli, nega yagona nosozlik nuqtasi (SPOF) emas, production uchun tavsiya etiladigan yaxshilanishlar: Keepalived, Prometheus, Kubernetes va h.k.).

### `QUICKSTART.md`
- Qisqacha versiyasi: skriptlarni bajariladigan qilish, `start.sh` bilan ishga tushirish, saytga kirish, 4 ta failover testi, to'xtatish, skriptlar jadvali, health check konfiguratsiyasi.
- Muhim faktlar: Load balancer 80-portda, backendlar 8001–8003 da, health check 3 ta muvaffaqiyatsizlikdan keyin, failover vaqti ~10–20 soniya, bir vaqtning o'zida 2 tagacha server nosozligini uzilishsiz ko'tara oladi.

### `MONITORING_GUIDE.md`
- `view/` papkasidagi grafik monitoring dashboard haqida to'liq qo'llanma.
- Dashboardni ishga tushirish (`control-scripts/start.sh` + `view/run.sh`), portlar jadvali (80, 8001-8003, 5000).
- Dashboard nimalarni ko'rsatishi: xost ma'lumotlari, kirish nuqtalari, load balancer holati, backend serverlar sog'lig'i, Docker konteynerlar holati, ishlash grafikalari.
- Failoverni dashboard orqali sinash stsenariylari (bitta/ikkita server to'xtatilganda va tiklanganda dashboard qanday o'zgarishini kutish kerakligi).
- Status belgilari (UP/DOWN/TIMEOUT), rang ko'rsatkichlari, grafikalarni talqin qilish bo'yicha tushuntirishlar.
- O'rnatish (`setup.sh` yoki qo'lda venv), nosozliklarni bartaraf etish.

### `SETUP_SUMMARY.txt`
- O'rnatish yakunlanganligi haqida qisqa (72 qatorli) xulosa hisoboti: qanday fayllar yaratilgan, tezkor start buyruqlari, arxitektura sxemasi (ASCII), load balancer konfiguratsiyasi xulosasi, tekshirilgan (verified) xususiyatlar ro'yxati.

### `view/README.md`
- `view/` dashboardining o'z ichidagi batafsil hujjati — o'rnatish (`pip install -r requirements.txt` yoki venv), ishga tushirish, dashboard bo'limlari tavsifi, ishlash ko'rsatkichlari jadvali, arxitektura vizualizatsiyasi, tizim talablari, portlar, nosozliklarni bartaraf etish. `README.md` (asosiy) va `MONITORING_GUIDE.md` bilan mazmuni katta qismda takrorlanadi/mos keladi.

### `view/USAGE_GUIDE.txt`
- Vizual (ASCII-art jadvalli) foydalanish qo'llanmasi — dashboardda nimalar ko'rinishi, rang ma'nolari, grafikalarni tushunish, to'liq ish oqimi namunasi (3 terminal + brauzer misoli bilan).

---

## 7. Portlar xulosasi

| Port | Xizmat | Kirish |
|------|--------|--------|
| 80 | Load Balancer (Nginx) | Asosiy kirish nuqtasi (`http://localhost`) |
| 8001–8007 | Backend web-serverlar (Nginx) | To'g'ridan-to'g'ri kirish/debugging uchun |
| 5000 | Monitoring & Management Dashboard (Flask) | `http://localhost:5000` |
| 22 | SSH (masofaviy xostlar uchun, `monitor_hosts.sh`/`ssh_login.sh` orqali) | Ixtiyoriy, masofaviy monitoring uchun |

---

## 8. Git holati (ushbu sessiya vaqtida)

- **Joriy branch:** `claude/busy-albattani-7yi10r`
- **Asosiy branch:** `main`
- **So'nggi commitlar** (eng yangisidan eskisiga):
  1. `bfdb6fb` — changes
  2. `0ca5ec8` — afd
  3. `2afd11b` — ax
  4. `c91c4f1` — fix check network and ports
  5. `ba98eca` — change view and control scripts, add nginx conf files, update docker-compose.yml and README.md
  6. `a30d130` — add all files (loyihaning boshlang'ich commiti)

---

## 9. E'tiborga olinishi kerak bo'lgan cheklovlar / texnik qarz

1. **Qattiq kodlangan yo'llar (hardcoded paths):** Deyarli barcha `control-scripts/*.sh` va `view/monitor.py` `PROJECT_DIR = "/home/akobir/Documents/Projects/DProjects/p1-3server"` ga bog'langan. Loyihani boshqa joyga ko'chirish yoki boshqa foydalanuvchi muhitida ishlatish uchun bu yo'lni yangilash kerak bo'ladi.
2. **`app.secret_key` ochiq matnda:** `view/monitor.py` faylida Flask session kaliti kodga qattiq yozilgan — production muhitida bu muhit o'zgaruvchisi orqali berilishi kerak.
3. **Konfiguratsiya fayllari ko'payib ketgan:** `nginx_8000.conf` dan `nginx_8099.conf` gacha bir nechta konfiguratsiya fayli (ba'zilari bir-biriga deyarli o'xshash, ba'zilari boshqa formatda) mavjud — bu turli vaqtlarda turli skriptlar (`start_servers.sh`, `add_server.sh`, `create_docker_server.sh`) tomonidan avtomatik yaratilganligi sababli.
4. **`nginx.conf.backup.*` fayllari:** 7 ta zaxira fayl repo ichida saqlanib qolgan (`update_load_balancer.sh` avtomatik yaratadi) — vaqt o'tishi bilan repo hajmini oshirishi mumkin, `.gitignore` ga qo'shish tavsiya etiladi.
5. **Health check faqat passive:** Nginx OSS versiyasi active health check (`health_check` direktivasi) ni qo'llab-quvvatlamaydi (bu faqat Nginx Plus'da bor) — shuning uchun tizim faqat passive (`max_fails`/`fail_timeout`) health checkka tayanadi, ya'ni haqiqiy foydalanuvchi so'rovi kelmaguncha server holati yangilanmaydi.
6. **`network_mode: host`:** Barcha konteynerlar host tarmog'ida ishlaydi (Docker bridge tarmog'i emas) — bu Linux'da yaxshi ishlaydi, lekin Docker Desktop (Mac/Windows) da `host` tarmoq rejimi cheklangan/qo'llab-quvvatlanmaydi, shu sabab loyiha asosan Linux muhiti uchun mo'ljallangan.

---

## 10. Tezkor buyruqlar (cheat-sheet)

```bash
# Infratuzilmani ishga tushirish (docker run asosida)
chmod +x control-scripts/*.sh
./control-scripts/start.sh

# yoki docker-compose orqali
docker-compose up -d

# Saytni tekshirish
curl http://localhost

# Bitta serverni to'xtatish / ishga tushirish
./control-scripts/stop1.sh
./control-scripts/start1.sh

# Barcha serverlarni boshqarish
./control-scripts/start_servers.sh all
./control-scripts/stop_servers.sh all
./control-scripts/restart_servers.sh all

# Yangi server qo'shish
./control-scripts/add_server.sh 4 8004

# Load balancer sozlamalari
./control-scripts/configure_load_balancer.sh list
./control-scripts/configure_load_balancer.sh set-method least_conn

# Monitoring dashboardni ishga tushirish
cd view && ./setup.sh   # bir martalik o'rnatish
cd view && ./run.sh     # http://localhost:5000

# Infratuzilmani to'xtatish
./control-scripts/stop.sh
# yoki
docker-compose down
```

---

*Ushbu fayl `claude/busy-albattani-7yi10r` branchida, loyihaning to'liq kodbazasini (barcha `.md`/`.txt` hujjatlar, `docker-compose.yml`, `nginx*.conf` fayllari, `control-scripts/*.sh` skriptlari va `view/` papkasidagi Flask ilovasi) o'rganib chiqish natijasida tuzilgan.*
