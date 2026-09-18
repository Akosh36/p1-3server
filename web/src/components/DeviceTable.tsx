import { useState } from 'react'
import type { AccessRole, Device, TrafficCapture } from '../api/types'
import { api } from '../api/client'
import { EmptyNote } from './Card'

interface Props {
  devices: Device[]
  role: AccessRole
  showCapture: boolean
  captures?: TrafficCapture[]
  onChanged: () => void
  onCaptureChanged?: () => void
}

function fmtElapsed(startedAt: string) {
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(startedAt).getTime()) / 1000))
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  return [h, m, s].map((v) => String(v).padStart(2, '0')).join(':')
}

// Shared table for the "Userlar" and "Adminlar" cards: both list devices
// holding the given access_grants role (nickname, device name, IP, MAC),
// let an admin rename a device or revoke its access, and — for users only —
// expose the on-demand pcap capture action (capd, Phase 6): start begins a
// real recording immediately, and while one is active the row shows how
// long it's been running and a Stop button instead. Downloading a finished
// file happens on the Logs page, where every capture segment is listed.
export default function DeviceTable({ devices, role, showCapture, captures, onChanged, onCaptureChanged }: Props) {
  const [editingId, setEditingId] = useState<number | null>(null)
  const [nickname, setNickname] = useState('')

  const filtered = devices.filter((d) => d.access_role === role)

  async function startCapture(deviceId: number) {
    await api.post(`/devices/${deviceId}/captures`)
    onCaptureChanged?.()
  }

  async function stopCapture(captureId: number) {
    await api.post(`/captures/${captureId}/stop`)
    onCaptureChanged?.()
  }

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
        {filtered.map((d) => {
          const activeCapture = captures?.find((c) => c.device_id === d.id && c.status === 'recording')
          return (
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
                activeCapture ? (
                  <span className="inline-flex items-center gap-2 mr-2">
                    <span className="text-xs font-mono" style={{ color: 'var(--danger)' }}>
                      ● {fmtElapsed(activeCapture.started_at)}
                    </span>
                    <button
                      onClick={() => stopCapture(activeCapture.id)}
                      className="px-2 py-1 rounded text-xs"
                      style={{ border: '1px solid var(--border)', color: 'var(--danger)' }}
                    >
                      ■ To'xtatish
                    </button>
                  </span>
                ) : (
                  <button
                    onClick={() => startCapture(d.id)}
                    title="Bu qurilmaning trafigini pcap formatida yozib olishni boshlaydi"
                    className="mr-2 px-2 py-1 rounded text-xs"
                    style={{ border: '1px solid var(--border)', color: 'var(--text)' }}
                  >
                    ● Trafik yozish
                  </button>
                )
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
          )
        })}
      </tbody>
    </table>
  )
}
