import { Fragment, useState } from 'react'
import type { AccessRole, Device, DeviceGroupTraffic, TrafficCapture } from '../api/types'
import { api } from '../api/client'
import { usePolling } from '../hooks/usePolling'
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

function fmtBytesTotal(v: number) {
  if (v > 1024 * 1024 * 1024) return `${(v / (1024 * 1024 * 1024)).toFixed(2)} GB`
  if (v > 1024 * 1024) return `${(v / (1024 * 1024)).toFixed(1)} MB`
  if (v > 1024) return `${(v / 1024).toFixed(1)} KB`
  return `${v} B`
}

// Which server groups this device has actually sent/received traffic
// through, and how much — populated by internal/lbsync resolving lbd's raw
// per-connection byte counts (Phase 8) against devices/server_groups. A
// device with no rows here simply hasn't used any load-balanced service
// yet, which is a normal state, not an error.
function DeviceTrafficDetail({ deviceId, colSpan }: { deviceId: number; colSpan: number }) {
  const { data: traffic } = usePolling(() => api.get<DeviceGroupTraffic[]>(`/devices/${deviceId}/traffic`), 10000)

  return (
    <tr style={{ borderTop: '1px solid var(--border)' }}>
      <td colSpan={colSpan} className="py-3">
        {!traffic || traffic.length === 0 ? (
          <EmptyNote>Bu qurilma hali hech qaysi server guruhi orqali trafik yubormagan.</EmptyNote>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr style={{ color: 'var(--text-muted)' }} className="text-left">
                <th className="pb-2 font-medium">Server guruhi</th>
                <th className="pb-2 font-medium">Yuklab olingan (↓)</th>
                <th className="pb-2 font-medium">Yuborilgan (↑)</th>
                <th className="pb-2 font-medium">Oxirgi faollik</th>
              </tr>
            </thead>
            <tbody>
              {traffic.map((t) => (
                <tr key={t.group_id} style={{ borderTop: '1px solid var(--border)' }}>
                  <td className="py-1 pr-2" style={{ color: 'var(--text)' }}>
                    {t.group_nickname}
                    <span className="text-xs ml-1" style={{ color: 'var(--text-muted)' }}>
                      ({t.vip_address}:{t.vip_port})
                    </span>
                  </td>
                  <td className="py-1 pr-2" style={{ color: 'var(--text)' }}>
                    {fmtBytesTotal(t.bytes_in)}
                  </td>
                  <td className="py-1 pr-2" style={{ color: 'var(--text)' }}>
                    {fmtBytesTotal(t.bytes_out)}
                  </td>
                  <td className="py-1 pr-2" style={{ color: 'var(--text-muted)' }}>
                    {t.last_activity_at ? new Date(t.last_activity_at).toLocaleString('uz-UZ') : '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </td>
    </tr>
  )
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
  const [trafficId, setTrafficId] = useState<number | null>(null)

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
          <Fragment key={d.id}>
          <tr style={{ borderTop: '1px solid var(--border)' }}>
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
                onClick={() => setTrafficId(trafficId === d.id ? null : d.id)}
                className="mr-2 px-2 py-1 rounded text-xs"
                style={{ border: '1px solid var(--border)', color: 'var(--text)' }}
              >
                {trafficId === d.id ? 'Trafikni yopish' : 'Trafik'}
              </button>
              <button
                onClick={() => revoke(d.id)}
                className="px-2 py-1 rounded text-xs"
                style={{ color: 'var(--danger)' }}
              >
                Huquqni olish
              </button>
            </td>
          </tr>
          {trafficId === d.id && <DeviceTrafficDetail deviceId={d.id} colSpan={6} />}
          </Fragment>
          )
        })}
      </tbody>
    </table>
  )
}
