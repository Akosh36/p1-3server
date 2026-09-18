// Package models defines the domain types shared by the API server and the
// data-plane daemons. They mirror the tables created by internal/db's
// embedded migrations.
package models

import "time"

type AdminRole string

const (
	RoleSuperAdmin AdminRole = "super_admin"
	RoleAdmin      AdminRole = "admin"
)

type AccessRole string

const (
	AccessUser  AccessRole = "user"
	AccessAdmin AccessRole = "admin"
)

type DeviceConnType string

const (
	ConnWired             DeviceConnType = "wired"
	ConnWirelessLocal     DeviceConnType = "wireless_local"
	ConnWirelessRemoteVPN DeviceConnType = "wireless_remote_vpn"
)

type LBAlgorithm string

const (
	AlgoRoundRobin LBAlgorithm = "round_robin"
	AlgoLeastConn  LBAlgorithm = "least_conn"
)

type CaptureStatus string

const (
	CaptureRecording  CaptureStatus = "recording"
	CaptureRotated    CaptureStatus = "rotated"
	CaptureDownloaded CaptureStatus = "downloaded"
	CaptureError      CaptureStatus = "error"
)

type Admin struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	TOTPSecret   *string    `json:"-"`
	Role         AdminRole  `json:"role"`
	AllowedIP    *string    `json:"allowed_ip,omitempty"`
	AllowedMAC   *string    `json:"allowed_mac,omitempty"`
	CreatedBy    *int64     `json:"created_by,omitempty"`
	IsActive     bool       `json:"is_active"`
	CreatedAt    time.Time  `json:"created_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

type Device struct {
	ID           int64          `json:"id"`
	MACAddress   string         `json:"mac_address"`
	IPAddress    *string        `json:"ip_address,omitempty"`
	Hostname     *string        `json:"hostname,omitempty"`
	Nickname     *string        `json:"nickname,omitempty"`
	ConnType     DeviceConnType `json:"conn_type"`
	SwitchPortID *int64         `json:"switch_port_id,omitempty"`
	SSID         *string        `json:"ssid,omitempty"`
	LANNetworkID *int64         `json:"lan_network_id,omitempty"`
	FirstSeenAt  time.Time      `json:"first_seen_at"`
	LastSeenAt   time.Time      `json:"last_seen_at"`
	IsOnline     bool           `json:"is_online"`

	// Populated by joins, not stored directly on this table.
	AccessRole *AccessRole `json:"access_role,omitempty"`
}

type AccessGrant struct {
	ID        int64      `json:"id"`
	DeviceID  int64      `json:"device_id"`
	Role      AccessRole `json:"role"`
	GrantedBy *int64     `json:"granted_by,omitempty"`
	GrantedAt time.Time  `json:"granted_at"`
}

type SwitchPort struct {
	ID           int64     `json:"id"`
	SwitchName   string    `json:"switch_name"`
	PortNumber   int       `json:"port_number"`
	Label        *string   `json:"label,omitempty"`
	VLAN         *int      `json:"vlan,omitempty"`
	LinkStatus   bool      `json:"link_status"`
	LastChangeAt time.Time `json:"last_change_at"`
}

type VPNPeer struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	PublicKey       string     `json:"public_key"`
	AllowedSubnet   string     `json:"allowed_subnet"`
	Endpoint        *string    `json:"endpoint,omitempty"`
	IsActive        bool       `json:"is_active"`
	LastHandshakeAt *time.Time `json:"last_handshake_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type LANNetwork struct {
	ID                int64          `json:"id"`
	Name              string         `json:"name"`
	Type              DeviceConnType `json:"type"`
	VPNPeerID         *int64         `json:"vpn_peer_id,omitempty"`
	IsActive          bool           `json:"is_active"`
	IsReachable       bool           `json:"is_reachable"`
	LastStatusCheckAt *time.Time     `json:"last_status_check_at,omitempty"`
}

type ServerGroup struct {
	ID         int64       `json:"id"`
	Nickname   string      `json:"nickname"`
	ColorHex   string      `json:"color_hex"`
	VIPAddress string      `json:"vip_address"`
	VIPPort    int         `json:"vip_port"`
	Protocol   string      `json:"protocol"`
	Algorithm  LBAlgorithm `json:"algorithm"`
	IsActive   bool        `json:"is_active"`
	CreatedAt  time.Time   `json:"created_at"`

	Backends []BackendServer `json:"backends,omitempty"`
}

type BackendServer struct {
	ID             int64      `json:"id"`
	GroupID        int64      `json:"group_id"`
	IP             string     `json:"ip"`
	Port           int        `json:"port"`
	Weight         int        `json:"weight"`
	IsHealthy      bool       `json:"is_healthy"`
	LastCheckAt    *time.Time `json:"last_check_at,omitempty"`
	ResponseTimeMs *float64   `json:"response_time_ms,omitempty"`
}

type TrafficCapture struct {
	ID               int64         `json:"id"`
	DeviceID         int64         `json:"device_id"`
	StartedByAdminID *int64        `json:"started_by_admin_id,omitempty"`
	FilePath         string        `json:"file_path"`
	StartedAt        time.Time     `json:"started_at"`
	StoppedAt        *time.Time    `json:"stopped_at,omitempty"`
	SizeBytes        int64         `json:"size_bytes"`
	Status           CaptureStatus `json:"status"`
	RotationReason   *string       `json:"rotation_reason,omitempty"`
}

type AuditLog struct {
	ID           int64                  `json:"id"`
	ActorAdminID *int64                 `json:"actor_admin_id,omitempty"`
	Action       string                 `json:"action"`
	TargetType   *string                `json:"target_type,omitempty"`
	TargetID     *int64                 `json:"target_id,omitempty"`
	Details      map[string]interface{} `json:"details,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
}

type SystemMetric struct {
	Time         time.Time `json:"time"`
	CPUPercent   float64   `json:"cpu_percent"`
	MemPercent   float64   `json:"mem_percent"`
	DiskReadBps  int64     `json:"disk_read_bps"`
	DiskWriteBps int64     `json:"disk_write_bps"`
	NetInBps     int64     `json:"net_in_bps"`
	NetOutBps    int64     `json:"net_out_bps"`
	DiskPercent  float64   `json:"disk_percent"`
}

type DeviceTrafficStat struct {
	Time           time.Time `json:"time"`
	DeviceID       int64     `json:"device_id"`
	BackendGroupID *int64    `json:"backend_group_id,omitempty"`
	BytesIn        int64     `json:"bytes_in"`
	BytesOut       int64     `json:"bytes_out"`
}
