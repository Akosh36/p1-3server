import { useState } from 'react'
import type { AccessRole, Device } from '../api/types'
import { api } from '../api/client'
import { EmptyNote } from './Card'

interface Props {
  devices: Device[]
  onChanged: () => void
}

const ROLE_LABEL: Record<string, string> = {
  '': 'Huquqsiz (bloklangan)',
  user: 'User',
  admin: 'Admin',
}

// The single place an admin actually grants User/Admin rights to a newly
// discovered device (per CLAUDE.md §2.1: "faqat adminlar IP/MAC bo'yicha
// qo'sha oladi"). Every device netdiscd has ever seen shows up here,
// regardless of its current role, so nothing found by Phase 1 is stranded
// without a way to act on it from the panel.
export default function AllDevicesTable({ devices, onChanged }: Props) {
  const [editingId, setEditingId] = useState<number | null>(null)
  const [nickname, setNickname] = useState('')

  async function saveNickname(id: number) {
    await api.patch(`/devices/${id}`, { nickname })
    setEditingId(null)
    onChanged()
  }

  async function setRole(id: number, role: AccessRole | '') {
    await api.patch(`/devices/${id}`, { access_role: role })
    onChanged()
  }

  if (devices.length === 0) {
    return (
      <EmptyNote>
        netdiscd hali hech qanday qurilma topmadi. Daemon ishga tushgach (ARP/DHCP/SNMP/hostapd orqali), qurilmalar shu yerda paydo bo'ladi.
      </EmptyNote>
    )
  }

  return (
    <table className="w-full text-sm">
      <thead>
        <tr style={{ color: 'var(--text-muted)' }} className="text-left">
          <th className="pb-2 font-medium">Nickname</th>
          <th className="pb-2 font-medium">Hostname</th>
          <th className="pb-2 font-medium">IP</th>
          <th className="pb-2 font-medium">MAC</th>
          <th className="pb-2 font-medium">Ulanish</th>
          <th className="pb-2 font-medium">Holat</th>
          <th className="pb-2 font-medium">Huquq</th>
        </tr>
      </thead>
      <tbody>
        {devices.map((d) => (
          <tr key={d.id} style={{ borderTop: '1px solid var(--border)' }}>
            <td className="py-2 pr-2" style={{ color: 'var(--text)' }}>
              {editingId === d.id ? (
                <input
                  autoFocus
                  className="px-2 py-1 rounded"
                  style={{ background: 'var(--surface-2)', border: '1px solid var(--border)' }}
                  value={nickname}
                  onChange={(e) => setNickname(e.target.value)}
                  onBlur={() => saveNickname(d.id)}
                  onKeyDown={(e) => e.key === 'Enter' && saveNickname(d.id)}
                />
              ) : (
                <button
                  className="hover:underline"
                  onClick={() => {
                    setEditingId(d.id)
                    setNickname(d.nickname ?? '')
                  }}
                >
                  {d.nickname || <span style={{ color: 'var(--text-muted)' }}>(nom yo'q — bosing)</span>}
                </button>
              )}
            </td>
            <td className="py-2 pr-2" style={{ color: 'var(--text)' }}>{d.hostname || '—'}</td>
            <td className="py-2 pr-2" style={{ color: 'var(--text)' }}>{d.ip_address || '—'}</td>
            <td className="py-2 pr-2 font-mono text-xs" style={{ color: 'var(--text-muted)' }}>{d.mac_address}</td>
            <td className="py-2 pr-2" style={{ color: 'var(--text)' }}>
              {d.conn_type === 'wireless_local' ? `WiFi${d.ssid ? ` (${d.ssid})` : ''}` : d.conn_type === 'wireless_remote_vpn' ? 'Masofaviy VPN' : 'Kabelli'}
            </td>
            <td className="py-2 pr-2">
              <span style={{ color: d.is_online ? 'var(--success)' : 'var(--text-muted)' }}>
                {d.is_online ? '● Onlayn' : '○ Offlayn'}
              </span>
            </td>
            <td className="py-2">
              <select
                value={d.access_role ?? ''}
                onChange={(e) => setRole(d.id, e.target.value as AccessRole | '')}
                className="px-2 py-1 rounded text-xs"
                style={{ background: 'var(--surface-2)', border: '1px solid var(--border)', color: 'var(--text)' }}
              >
                <option value="">{ROLE_LABEL['']}</option>
                <option value="user">{ROLE_LABEL['user']}</option>
                <option value="admin">{ROLE_LABEL['admin']}</option>
              </select>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
