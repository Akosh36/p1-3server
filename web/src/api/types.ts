export type AdminRole = 'super_admin' | 'admin'
export type AccessRole = 'user' | 'admin'
export type ConnType = 'wired' | 'wireless_local' | 'wireless_remote_vpn'
export type LBAlgorithm = 'round_robin' | 'least_conn'

export interface Admin {
  id: number
  username: string
  role: AdminRole
  allowed_ip?: string
  allowed_mac?: string
  is_active: boolean
  created_at: string
  last_login_at?: string
}

export interface Device {
  id: number
  mac_address: string
  ip_address?: string
  hostname?: string
  nickname?: string
  conn_type: ConnType
  switch_port_id?: number
  ssid?: string
  lan_network_id?: number
  first_seen_at: string
  last_seen_at: string
  is_online: boolean
  access_role?: AccessRole
}

export interface SwitchPort {
  id: number
  switch_name: string
  port_number: number
  label?: string
  vlan?: number
  link_status: boolean
  last_change_at: string
}

export interface LANNetwork {
  id: number
  name: string
  type: ConnType
  vpn_peer_id?: number
  is_active: boolean
  is_reachable: boolean
  last_status_check_at?: string
  vpn_public_key?: string
  vpn_allowed_subnet?: string
  vpn_endpoint?: string
  vpn_last_handshake_at?: string
}

export interface WireGuardLocalInfo {
  public_key: string
  listen_port: number
}

export interface BackendServer {
  id: number
  group_id: number
  ip: string
  port: number
  weight: number
  is_healthy: boolean
  last_check_at?: string
  response_time_ms?: number
  agent_token?: string
}

export interface BackendMetric {
  time: string
  cpu_percent: number
  mem_percent: number
  disk_percent: number
  disk_read_bps: number
  disk_write_bps: number
  net_in_bps: number
  net_out_bps: number
  gpu_percent?: number
  gpu_mem_percent?: number
}

export interface ServerGroup {
  id: number
  nickname: string
  color_hex: string
  vip_address: string
  vip_port: number
  protocol: string
  algorithm: LBAlgorithm
  is_active: boolean
  created_at: string
  backends: BackendServer[]
}

export interface SystemMetric {
  time: string
  cpu_percent: number
  mem_percent: number
  disk_read_bps: number
  disk_write_bps: number
  net_in_bps: number
  net_out_bps: number
  disk_percent: number
  gpu_percent?: number
  gpu_mem_percent?: number
}

export interface FirewallStatus {
  connected: boolean
  user_mac_count: number
  admin_mac_count: number
  last_applied_at?: string
  last_error?: string
}

export interface AuditLog {
  id: number
  actor_admin_id?: number
  action: string
  target_type?: string
  target_id?: number
  details?: Record<string, unknown>
  created_at: string
}

export interface DeviceGroupTraffic {
  group_id: number
  group_nickname: string
  vip_address: string
  vip_port: number
  bytes_in: number
  bytes_out: number
  last_activity_at?: string
}

export type CaptureStatus = 'recording' | 'rotated' | 'downloaded' | 'error' | 'stopped'

export interface TrafficCapture {
  id: number
  device_id: number
  device_nickname?: string
  device_mac: string
  started_by_admin_id?: number
  file_path: string
  started_at: string
  stopped_at?: string
  size_bytes: number
  status: CaptureStatus
  rotation_reason?: string
}
