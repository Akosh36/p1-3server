import { NavLink, Outlet } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'

const NAV_ITEMS = [
  { to: '/', label: 'Bosh sahifa', end: true },
  { to: '/server', label: 'Server' },
  { to: '/users', label: 'Userlar' },
  { to: '/admins', label: 'Adminlar' },
  { to: '/servers', label: 'Serverlar' },
  { to: '/lan', label: 'LAN' },
  { to: '/firewall', label: 'Firewall' },
  { to: '/logs', label: 'Logs' },
]

export default function Layout() {
  const { admin, logout } = useAuth()

  return (
    <div className="min-h-screen flex" style={{ background: 'var(--bg)' }}>
      <aside
        className="w-56 shrink-0 p-4 flex flex-col gap-1"
        style={{ background: 'var(--surface)', borderRight: '1px solid var(--border)' }}
      >
        <div className="px-2 py-3 mb-2">
          <div className="font-semibold" style={{ color: 'var(--text)' }}>
            LAN Control
          </div>
          <div className="text-xs" style={{ color: 'var(--text-muted)' }}>
            {admin?.role === 'super_admin' ? 'Super-admin' : 'Admin'} · {admin?.username}
          </div>
        </div>

        {NAV_ITEMS.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            className={({ isActive }) =>
              `px-3 py-2 rounded-lg text-sm font-medium transition-colors ${isActive ? '' : 'opacity-70 hover:opacity-100'}`
            }
            style={({ isActive }) => ({
              background: isActive ? 'var(--surface-2)' : 'transparent',
              color: isActive ? 'var(--accent)' : 'var(--text)',
            })}
          >
            {item.label}
          </NavLink>
        ))}

        <button
          onClick={logout}
          className="mt-auto px-3 py-2 rounded-lg text-sm text-left"
          style={{ color: 'var(--danger)' }}
        >
          Chiqish
        </button>
      </aside>

      <main className="flex-1 p-6 overflow-y-auto">
        <Outlet />
      </main>
    </div>
  )
}
