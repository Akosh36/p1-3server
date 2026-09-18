import { useState } from 'react'
import type { AccessRole, Device } from '../api/types'
import { api } from '../api/client'
import { EmptyNote } from './Card'

interface Props {
  devices: Device[]
  role: AccessRole
  showCapture: boolean
  onChanged: () => void
}

// Shared table for the "Userlar" and "Adminlar" cards: both list devices
// holding the given access_grants role (nickname, device name, IP, MAC),
// let an admin rename a device or revoke its access, and — for users only —
// expose the on-demand pcap capture action (wired up once capd/Phase 6
// exists; the button is present but disabled with an honest note until then).
export default function DeviceTable({ devices, role, showCapture, onChanged }: Props) {
  const [editingId, setEditingId] = useState<number | null>(null)
  const [nickname, setNickname] = useState('')

  const filtered = devices.filter((d) => d.access_role === role)

  async function saveNickname(id: number) {
    await api.patch(`/devices/${id}`, { nickname })
    setEditingId(null)
    onChanged()
  }

  async function revoke(id: number) {
    if (!confirm('Bu qurilmaning huquqini olib tashlaysizmi? U keyin hech qayerga kira olmaydi.')) return
    await api.patch(`/devices/${id}`, { access_role: '' })
    onChanged()
  }

  if (filtered.length === 0) {
    return <EmptyNote>Hozircha bu guruhda qurilma yo'q. Qurilmalar LAN sahifasida topiladi va shu yerga huquq berish orqali qo'shiladi.</EmptyNote>
  }

  return (
    <table className="w-full text-sm">
      <thead>
        <tr style={{ color: 'var(--text-muted)' }} className="text-left">
          <th className="pb-2 font-medium">Nickname</th>
          <th className="pb-2 font-medium">Qurilma nomi</th>
          <th className="pb-2 font-medium">IP</th>
          <th className="pb-2 font-medium">MAC</th>
          <th className="pb-2 font-medium">Holat</th>
          <th className="pb-2 font-medium"></th>
        </tr>
      </thead>
      <tbody>
        {filtered.map((d) => (
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
            <td className="py-2 pr-2" style={{ color: 'var(--text)' }}>
              {d.hostname || '—'}
            </td>
            <td className="py-2 pr-2" style={{ color: 'var(--text)' }}>
              {d.ip_address || '—'}
            </td>
            <td className="py-2 pr-2 font-mono text-xs" style={{ color: 'var(--text-muted)' }}>
              {d.mac_address}
            </td>
            <td className="py-2 pr-2">
              <span
                className="px-2 py-0.5 rounded-full text-xs font-medium"
                style={{
                  background: d.is_online ? 'color-mix(in srgb, var(--success) 15%, transparent)' : 'color-mix(in srgb, var(--text-muted) 15%, transparent)',
                  color: d.is_online ? 'var(--success)' : 'var(--text-muted)',
                }}
              >
                {d.is_online ? 'Onlayn' : 'Offlayn'}
              </span>
            </td>
            <td className="py-2 text-right whitespace-nowrap">
              {showCapture && (
                <button
                  title="Trafikni yozib olish — capd daemoni (Phase 6) qo'shilgach ishlaydi"
                  disabled
                  className="mr-2 px-2 py-1 rounded text-xs opacity-50 cursor-not-allowed"
                  style={{ border: '1px solid var(--border)', color: 'var(--text-muted)' }}
                >
                  ● Trafik yozish
                </button>
              )}
              <button
                onClick={() => revoke(d.id)}
                className="px-2 py-1 rounded text-xs"
                style={{ color: 'var(--danger)' }}
              >
                Huquqni olish
              </button>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
