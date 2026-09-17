import { usePolling } from '../hooks/usePolling'
import { api } from '../api/client'
import type { Device } from '../api/types'
import { Panel, StatTile, EmptyNote } from '../components/Card'

// fwctl (Phase 2) is not wired in yet, so this page shows what is real right
// now: how many discovered devices are granted vs. blocked by default-deny.
// Once fwctl exists it will additionally show live nftables counters here.
export default function FirewallPage() {
  const { data: devices } = usePolling(() => api.get<Device[]>('/devices'))

  const total = devices?.length ?? 0
  const users = devices?.filter((d) => d.access_role === 'user').length ?? 0
  const admins = devices?.filter((d) => d.access_role === 'admin').length ?? 0
  const blocked = total - users - admins

  return (
    <div>
      <h1 className="text-xl font-semibold mb-4" style={{ color: 'var(--text)' }}>
        Firewall
      </h1>

      <Panel title="Kirish nazorati holati">
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mb-4">
          <StatTile label="Jami topilgan qurilma" value={total} />
          <StatTile label="User huquqi" value={users} />
          <StatTile label="Admin huquqi" value={admins} />
          <StatTile label="Bloklangan (huquqsiz)" value={blocked} hint="default-deny" />
        </div>
        <EmptyNote>
          nftables sinxronizatsiyasi (fwctl, Phase 2) hali ulanmagan — hozircha bu son ma'lumotlar bazasidagi
          access_grants holatini ko'rsatadi. fwctl ishga tushgach, bu yerda real-vaqt DDoS himoyasi statistikasi
          (rad etilgan paketlar soni, rate-limit hodisalari) ham chiqadi.
        </EmptyNote>
      </Panel>
    </div>
  )
}
