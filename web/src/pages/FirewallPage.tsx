import { usePolling } from '../hooks/usePolling'
import { api } from '../api/client'
import type { Device, FirewallStatus } from '../api/types'
import { Panel, StatTile, EmptyNote } from '../components/Card'

export default function FirewallPage() {
  const { data: devices } = usePolling(() => api.get<Device[]>('/devices'))
  const { data: fw } = usePolling(() => api.get<FirewallStatus>('/firewall/status'), 3000)

  const total = devices?.length ?? 0
  const users = devices?.filter((d) => d.access_role === 'user').length ?? 0
  const admins = devices?.filter((d) => d.access_role === 'admin').length ?? 0
  const blocked = total - users - admins

  return (
    <div>
      <h1 className="text-xl font-semibold mb-4" style={{ color: 'var(--text)' }}>
        Firewall
      </h1>

      <Panel title="Kirish nazorati holati (ma'lumotlar bazasi — nima bo'lishi kerak)">
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
          <StatTile label="Jami topilgan qurilma" value={total} />
          <StatTile label="User huquqi" value={users} />
          <StatTile label="Admin huquqi" value={admins} />
          <StatTile label="Bloklangan (huquqsiz)" value={blocked} hint="default-deny" />
        </div>
      </Panel>

      <Panel title="fwctl / nftables holati (haqiqiy — hozir kernelda yuklangan)">
        {!fw?.connected ? (
          <EmptyNote>
            fwctl daemoniga ulanib bo'lmadi ({'/run/p13server/fwctl.sock'}) — u ishga tushgan bo'lishi kerak
            (production'da systemd orqali, `deploy/systemd/fwctl.service`). U ishlamasa, tarmoq darajasida
            hech qanday kirish nazorati amalda emas.
          </EmptyNote>
        ) : (
          <>
            <div className="grid grid-cols-2 md:grid-cols-3 gap-3 mb-3">
              <StatTile
                label="Ulanish"
                value={<span style={{ color: 'var(--success)' }}>● Faol</span>}
              />
              <StatTile label="Kernelda user MAC" value={fw.user_mac_count} />
              <StatTile label="Kernelda admin MAC" value={fw.admin_mac_count} />
            </div>
            <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
              Oxirgi qo'llangan: {fw.last_applied_at ? new Date(fw.last_applied_at).toLocaleString('uz-UZ') : '—'}
              {' · '}Sinxronizatsiya har ~2 soniyada (aclsync)
            </p>
            {fw.last_error && (
              <p className="text-sm mt-2" style={{ color: 'var(--danger)' }}>
                Oxirgi xato: {fw.last_error}
              </p>
            )}
          </>
        )}
        <p className="text-xs mt-3" style={{ color: 'var(--text-muted)' }}>
          DDoS himoyasi: yangi ulanishlar har bir MAC/IP uchun cheklangan (admin 100/s, user 50/s, boshqaruv
          portiga SYN 20/s) — nftables dinamik meter'lari orqali, `internal/firewall/ruleset.go`da hujjatlashtirilgan.
        </p>
      </Panel>
    </div>
  )
}
