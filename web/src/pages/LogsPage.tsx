import { usePolling } from '../hooks/usePolling'
import { api } from '../api/client'
import type { AuditLog } from '../api/types'
import { Panel, EmptyNote } from '../components/Card'

export default function LogsPage() {
  const { data: logs } = usePolling(() => api.get<AuditLog[]>('/audit-logs'), 8000)

  return (
    <div>
      <h1 className="text-xl font-semibold mb-4" style={{ color: 'var(--text)' }}>
        Logs
      </h1>

      <Panel title="Foydalanuvchi trafigi (pcap) yozuvlari">
        <EmptyNote>
          Bu bo'lim capd daemoni (Phase 6) qo'shilgach ishga tushadi: har bir yozuv uchun user, fayl qancha vaqtdan
          beri yozilayotgani va "Download" tugmasi (bosilganda yozuv yakunlanib yuklanadi, so'ng yangi faylga
          yozish davom etadi) shu yerda ko'rinadi.
        </EmptyNote>
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
