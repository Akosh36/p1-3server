-- Core schema for the LAN access-control & load-balancing platform.
-- Plain PostgreSQL; system_metrics/device_traffic_stats are written so that
-- `SELECT create_hypertable(...)` can be applied later if TimescaleDB is
-- installed in production, without changing this schema.

CREATE TYPE admin_role AS ENUM ('super_admin', 'admin');
CREATE TYPE device_conn_type AS ENUM ('wired', 'wireless_local', 'wireless_remote_vpn');
CREATE TYPE access_role AS ENUM ('user', 'admin');
CREATE TYPE lb_algorithm AS ENUM ('round_robin', 'least_conn');
CREATE TYPE capture_status AS ENUM ('recording', 'rotated', 'downloaded', 'error');
CREATE TYPE lan_type AS ENUM ('wired', 'wireless_local', 'wireless_remote_vpn');

-- ---------------------------------------------------------------------------
-- Admins (super_admin manages admin accounts; admin manages users/devices)
-- ---------------------------------------------------------------------------
CREATE TABLE admins (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username        TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    totp_secret     TEXT,                         -- NULL = 2FA not enabled
    role            admin_role NOT NULL DEFAULT 'admin',
    allowed_ip      INET,                         -- optional extra IP pin
    allowed_mac     MACADDR,                      -- optional extra MAC pin
    created_by      BIGINT REFERENCES admins(id) ON DELETE SET NULL,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at   TIMESTAMPTZ
);

-- ---------------------------------------------------------------------------
-- Devices seen on the LAN (populated by netdiscd)
-- ---------------------------------------------------------------------------
CREATE TABLE devices (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    mac_address     MACADDR NOT NULL UNIQUE,
    ip_address      INET,
    hostname        TEXT,                         -- reported by DHCP/mDNS
    nickname        TEXT,                         -- admin-assigned label
    conn_type       device_conn_type NOT NULL DEFAULT 'wired',
    switch_port_id  BIGINT,                        -- FK added after switch_ports exists
    ssid            TEXT,                          -- for wireless_local
    lan_network_id  BIGINT,                        -- FK added after lan_networks exists
    first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    is_online       BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX idx_devices_ip ON devices(ip_address);
CREATE INDEX idx_devices_online ON devices(is_online);

-- ---------------------------------------------------------------------------
-- Access grants: the ONLY source of truth for who is "user" vs "admin".
-- A device with no row here is blocked by default (default-deny firewall).
-- ---------------------------------------------------------------------------
CREATE TABLE access_grants (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    device_id       BIGINT NOT NULL UNIQUE REFERENCES devices(id) ON DELETE CASCADE,
    role            access_role NOT NULL,
    granted_by      BIGINT REFERENCES admins(id) ON DELETE SET NULL,
    granted_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_access_grants_role ON access_grants(role);

-- ---------------------------------------------------------------------------
-- Managed switch ports (wired LAN table)
-- ---------------------------------------------------------------------------
CREATE TABLE switch_ports (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    switch_name     TEXT NOT NULL,
    port_number     INTEGER NOT NULL,
    label           TEXT,
    vlan            INTEGER,
    link_status     BOOLEAN NOT NULL DEFAULT FALSE,
    last_change_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (switch_name, port_number)
);

ALTER TABLE devices
    ADD CONSTRAINT fk_devices_switch_port
    FOREIGN KEY (switch_port_id) REFERENCES switch_ports(id) ON DELETE SET NULL;

-- ---------------------------------------------------------------------------
-- LAN networks: local wireless AP + remote wireless LANs reached over VPN
-- ---------------------------------------------------------------------------
CREATE TABLE vpn_peers (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name                TEXT NOT NULL UNIQUE,
    public_key          TEXT NOT NULL,
    allowed_subnet      CIDR NOT NULL,
    endpoint            TEXT,                     -- host:port of the remote peer
    is_active           BOOLEAN NOT NULL DEFAULT TRUE,
    last_handshake_at   TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE lan_networks (
    id                      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name                    TEXT NOT NULL,
    type                    lan_type NOT NULL,
    vpn_peer_id             BIGINT REFERENCES vpn_peers(id) ON DELETE SET NULL,
    is_active               BOOLEAN NOT NULL DEFAULT TRUE,
    is_reachable            BOOLEAN NOT NULL DEFAULT FALSE,
    last_status_check_at    TIMESTAMPTZ
);

ALTER TABLE devices
    ADD CONSTRAINT fk_devices_lan_network
    FOREIGN KEY (lan_network_id) REFERENCES lan_networks(id) ON DELETE SET NULL;

-- ---------------------------------------------------------------------------
-- Load-balanced backend server groups (each gets its own VIP)
-- ---------------------------------------------------------------------------
CREATE TABLE server_groups (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    nickname        TEXT NOT NULL UNIQUE,
    color_hex       TEXT NOT NULL DEFAULT '#4f46e5',
    vip_address     INET NOT NULL UNIQUE,
    vip_port        INTEGER NOT NULL,
    protocol        TEXT NOT NULL DEFAULT 'tcp',
    algorithm       lb_algorithm NOT NULL DEFAULT 'round_robin',
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE backend_servers (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    group_id            BIGINT NOT NULL REFERENCES server_groups(id) ON DELETE CASCADE,
    ip                  INET NOT NULL,
    port                INTEGER NOT NULL,
    weight              INTEGER NOT NULL DEFAULT 1,
    is_healthy          BOOLEAN NOT NULL DEFAULT FALSE,
    last_check_at       TIMESTAMPTZ,
    response_time_ms    NUMERIC,
    UNIQUE (group_id, ip, port)
);

-- ---------------------------------------------------------------------------
-- On-demand per-device packet capture sessions (Logs page)
-- ---------------------------------------------------------------------------
CREATE TABLE traffic_captures (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    device_id           BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    started_by_admin_id BIGINT REFERENCES admins(id) ON DELETE SET NULL,
    file_path           TEXT NOT NULL,
    started_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    stopped_at          TIMESTAMPTZ,
    size_bytes          BIGINT NOT NULL DEFAULT 0,
    status              capture_status NOT NULL DEFAULT 'recording',
    rotation_reason     TEXT
);
CREATE INDEX idx_traffic_captures_device ON traffic_captures(device_id);
CREATE INDEX idx_traffic_captures_status ON traffic_captures(status);

-- ---------------------------------------------------------------------------
-- Audit log (every admin action)
-- ---------------------------------------------------------------------------
CREATE TABLE audit_logs (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_admin_id  BIGINT REFERENCES admins(id) ON DELETE SET NULL,
    action          TEXT NOT NULL,
    target_type     TEXT,
    target_id       BIGINT,
    details         JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at DESC);

-- ---------------------------------------------------------------------------
-- Time-series metrics (candidate hypertables in production)
-- ---------------------------------------------------------------------------
CREATE TABLE system_metrics (
    time            TIMESTAMPTZ NOT NULL DEFAULT now(),
    cpu_percent     NUMERIC,
    mem_percent     NUMERIC,
    disk_read_bps   BIGINT,
    disk_write_bps  BIGINT,
    net_in_bps      BIGINT,
    net_out_bps     BIGINT,
    disk_percent    NUMERIC
);
CREATE INDEX idx_system_metrics_time ON system_metrics(time DESC);

CREATE TABLE device_traffic_stats (
    time                TIMESTAMPTZ NOT NULL DEFAULT now(),
    device_id           BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    backend_group_id    BIGINT REFERENCES server_groups(id) ON DELETE SET NULL,
    bytes_in            BIGINT NOT NULL DEFAULT 0,
    bytes_out           BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX idx_device_traffic_stats_time ON device_traffic_stats(time DESC);
CREATE INDEX idx_device_traffic_stats_device ON device_traffic_stats(device_id, time DESC);
