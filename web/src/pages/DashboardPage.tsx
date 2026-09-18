import { Link } from 'react-router-dom'
import { usePolling } from '../hooks/usePolling'
import { api } from '../api/client'
import type { Device, LANNetwork, ServerGroup } from '../api/types'

interface CardDef {
  to: string
  title: string
  value: string
  hint: string
}

export default function DashboardPage() {
  const { data: devices } = usePolling(() => api.get<Device[]>('/devices'))
  const { data: groups } = usePolling(() => api.get<ServerGroup[]>('/server-groups'))
  const { data: networks } = usePolling(() => api.get<LANNetwork[]>('/lan-networks'))

  const users = devices?.filter((d) => d.access_role === 'user').length ?? 0
  const admins = devices?.filter((d) => d.access_role === 'admin').length ?? 0
  const online = devices?.filter((d) => d.is_online).length ?? 0

  const cards: CardDef[] = [
    { to: '/server', title: 'Server', value: '—', hint: 'CPU / RAM / Disk / Tarmoq' },
    { to: '/users', title: 'Userlar', value: String(users), hint: "'user' huquqli qurilmalar" },
    { to: '/admins', title: 'Adminlar', value: String(admins), hint: "'admin' huquqli qurilmalar" },
    { to: '/servers', title: 'Serverlar', value: String(groups?.length ?? 0), hint: 'Load balancing guruhlari' },
    { to: '/lan', title: 'LAN', value: String(networks?.length ?? 0), hint: `${online} qurilma onlayn` },
    { to: '/firewall', title: 'Firewall', value: String((devices?.length ?? 0) - users - admins), hint: 'Bloklangan qurilmalar' },
    { to: '/logs', title: 'Logs', value: '—', hint: 'Trafik yozuvlari va hodisalar' },
  ]

  return (
    <div>
      <h1 className="text-xl font-semibold mb-1" style={{ color: 'var(--text)' }}>
        Bosh sahifa
      </h1>
      <p className="text-sm mb-6" style={{ color: 'var(--text-muted)' }}>
        LAN tarmog'ini boshqarish uchun bo'lim tanlang
      </p>

      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
        {cards.map((c) => (
          <Link
            key={c.to}
            to={c.to}
            className="rounded-2xl p-5 aspect-square flex flex-col justify-between hover:-translate-y-0.5 transition-transform"
            style={{ background: 'var(--surface)', border: '1px solid var(--border)' }}
          >
            <div className="font-semibold" style={{ color: 'var(--text)' }}>
              {c.title}
            </div>
            <div>
              <div className="text-3xl font-bold" style={{ color: 'var(--accent)' }}>
                {c.value}
              </div>
              <div className="text-xs mt-1" style={{ color: 'var(--text-muted)' }}>
                {c.hint}
              </div>
            </div>
          </Link>
        ))}
      </div>
    </div>
  )
}
