import { usePolling } from '../hooks/usePolling'
import { api, downloadFile } from '../api/client'
import type { AuditLog, CaptureStatus, TrafficCapture } from '../api/types'
import { Panel, EmptyNote } from '../components/Card'

function fmtBytes(v: number) {
  if (v > 1024 * 1024) return `${(v / (1024 * 1024)).toFixed(1)} MB`
  if (v > 1024) return `${(v / 1024).toFixed(1)} KB`
  return `${v} B`
}

function fmtDuration(startedAt: string, stoppedAt?: string) {
  const end = stoppedAt ? new Date(stoppedAt).getTime() : Date.now()
  const seconds = Math.max(0, Math.floor((end - new Date(startedAt).getTime()) / 1000))
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  return [h, m, s].map((v) => String(v).padStart(2, '0')).join(':')
}

const STATUS_LABEL: Record<CaptureStatus, string> = {
  recording: '● Yozilmoqda',
  rotated: '↻ Avtomatik yangilandi',
  downloaded: '↓ Yuklab olindi',
  stopped: '■ To\'xtatildi',
  error: '⚠ Xato',
}

function statusColor(status: CaptureStatus) {
  if (status === 'recording') return 'var(--danger)'
  if (status === 'error') return 'var(--danger)'
  if (status === 'downloaded') return 'var(--success)'
  return 'var(--text-muted)'
}

const REASON_LABEL: Record<string, string> = {
  size_limit: 'fayl hajmi chegarasi',
  time_limit: 'vaqt chegarasi',
  manual_download: "qo'lda yuklab olindi",
  quota_evicted: "disk kvotasi tufayli o'chirildi",
}

export default function LogsPage() {
  const { data: captures, refresh: refreshCaptures } = usePolling(() => api.get<TrafficCapture[]>('/captures'), 4000)
  const { data: logs } = usePolling(() => api.get<AuditLog[]>('/audit-logs'), 8000)

  async function download(id: number) {
    try {
      const { blob, filename } = await downloadFile(`/captures/${id}/download`)
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = filename
      a.click()
      URL.revokeObjectURL(url)
    } finally {
      refreshCaptures()
    }
  }

  return (
    <div>
      <h1 className="text-xl font-semibold mb-4" style={{ color: 'var(--text)' }}>
        Logs
      </h1>

      <Panel title="Foydalanuvchi trafigi (pcap) yozuvlari">
        {(captures ?? []).length === 0 ? (
          <EmptyNote>
            Hali yozuv yo'q. Userlar sahifasida biror qurilma qatoridagi "● Trafik yozish" tugmasini bosing.
          </EmptyNote>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr style={{ color: 'var(--text-muted)' }} className="text-left">
                <th className="pb-2 font-medium">Qurilma</th>
                <th className="pb-2 font-medium">Boshlangan</th>
                <th className="pb-2 font-medium">Davomiyligi</th>
                <th className="pb-2 font-medium">Hajmi</th>
                <th className="pb-2 font-medium">Holat</th>
                <th className="pb-2"></th>
              </tr>
            </thead>
            <tbody>
              {(captures ?? []).map((c) => (
                <tr key={c.id} style={{ borderTop: '1px solid var(--border)' }}>
                  <td className="py-2" style={{ color: 'var(--text)' }}>
                    {c.device_nickname || <span className="font-mono text-xs" style={{ color: 'var(--text-muted)' }}>{c.device_mac}</span>}
                  </td>
                  <td className="py-2 whitespace-nowrap" style={{ color: 'var(--text-muted)' }}>
                    {new Date(c.started_at).toLocaleString('uz-UZ')}
                  </td>
                  <td className="py-2 font-mono" style={{ color: 'var(--text)' }}>
                    {fmtDuration(c.started_at, c.stopped_at)}
                    {c.status === 'recording' && (
                      <span className="ml-1 text-xs" style={{ color: 'var(--text-muted)' }}>
                        (davom etmoqda)
                      </span>
                    )}
                  </td>
                  <td className="py-2" style={{ color: 'var(--text)' }}>
                    {fmtBytes(c.size_bytes)}
                  </td>
                  <td className="py-2">
                    <span style={{ color: statusColor(c.status) }}>{STATUS_LABEL[c.status]}</span>
                    {c.rotation_reason && (
                      <span className="ml-1 text-xs" style={{ color: 'var(--text-muted)' }}>
                        ({REASON_LABEL[c.rotation_reason] ?? c.rotation_reason})
                      </span>
                    )}
                  </td>
                  <td className="py-2 text-right">
                    {c.status !== 'error' && (
                      <button onClick={() => download(c.id)} className="text-xs" style={{ color: 'var(--accent)' }}>
                        Yuklab olish
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Panel>

      <Panel title="Tizim hodisalari (audit log)">
        {(logs ?? []).length === 0 ? (
          <EmptyNote>Hali hodisa qayd etilmagan.</EmptyNote>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr style={{ color: 'var(--text-muted)' }} className="text-left">
                <th className="pb-2 font-medium">Vaqt</th>
                <th className="pb-2 font-medium">Amal</th>
                <th className="pb-2 font-medium">Nishon</th>
                <th className="pb-2 font-medium">Tafsilot</th>
              </tr>
            </thead>
            <tbody>
              {(logs ?? []).map((l) => (
                <tr key={l.id} style={{ borderTop: '1px solid var(--border)' }}>
                  <td className="py-2 whitespace-nowrap" style={{ color: 'var(--text-muted)' }}>
                    {new Date(l.created_at).toLocaleString('uz-UZ')}
                  </td>
                  <td className="py-2 font-mono text-xs" style={{ color: 'var(--text)' }}>
                    {l.action}
                  </td>
                  <td className="py-2" style={{ color: 'var(--text)' }}>
                    {l.target_type ? `${l.target_type}#${l.target_id}` : '—'}
                  </td>
                  <td className="py-2 font-mono text-xs" style={{ color: 'var(--text-muted)' }}>
                    {l.details ? JSON.stringify(l.details) : ''}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Panel>
    </div>
  )
}
