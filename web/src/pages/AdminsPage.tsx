import { useState, type FormEvent } from 'react'
import { usePolling } from '../hooks/usePolling'
import { api } from '../api/client'
import type { Admin, Device } from '../api/types'
import { Panel, EmptyNote } from '../components/Card'
import DeviceTable from '../components/DeviceTable'
import { useAuth } from '../context/AuthContext'

export default function AdminsPage() {
  const { admin: currentAdmin } = useAuth()
  const { data: devices, refresh: refreshDevices } = usePolling(() => api.get<Device[]>('/devices'))
  const { data: panelAccounts, refresh: refreshAccounts } = usePolling(
    () => api.get<Admin[]>('/admins'),
    10000,
  )

  return (
    <div>
      <h1 className="text-xl font-semibold mb-4" style={{ color: 'var(--text)' }}>
        Adminlar
      </h1>

      <Panel title="'Admin' tarmoq huquqiga ega qurilmalar">
        <DeviceTable devices={devices ?? []} role="admin" showCapture={false} onChanged={refreshDevices} />
      </Panel>

      {currentAdmin?.role === 'super_admin' && (
        <Panel title="Panel kirish hisoblari (login/parol)">
          <PanelAccounts accounts={panelAccounts ?? []} onChanged={refreshAccounts} />
        </Panel>
      )}
    </div>
  )
}

function PanelAccounts({ accounts, onChanged }: { accounts: Admin[]; onChanged: () => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState<'admin' | 'super_admin'>('admin')
  const [error, setError] = useState<string | null>(null)

  async function create(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await api.post('/admins', { username, password, role })
      setUsername('')
      setPassword('')
      onChanged()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create admin')
    }
  }

  async function remove(id: number) {
    if (!confirm("Bu panel hisobini o'chirasizmi?")) return
    await api.delete(`/admins/${id}`)
    onChanged()
  }

  return (
    <div>
      {accounts.length === 0 ? (
        <EmptyNote>Hisoblar yuklanmoqda...</EmptyNote>
      ) : (
        <table className="w-full text-sm mb-4">
          <thead>
            <tr style={{ color: 'var(--text-muted)' }} className="text-left">
              <th className="pb-2 font-medium">Login</th>
              <th className="pb-2 font-medium">Rol</th>
              <th className="pb-2 font-medium">Oxirgi kirish</th>
              <th className="pb-2"></th>
            </tr>
          </thead>
          <tbody>
            {accounts.map((a) => (
              <tr key={a.id} style={{ borderTop: '1px solid var(--border)' }}>
                <td className="py-2" style={{ color: 'var(--text)' }}>
                  {a.username}
                </td>
                <td className="py-2" style={{ color: 'var(--text)' }}>
                  {a.role === 'super_admin' ? 'Super-admin' : 'Admin'}
                </td>
                <td className="py-2" style={{ color: 'var(--text-muted)' }}>
                  {a.last_login_at ? new Date(a.last_login_at).toLocaleString('uz-UZ') : '—'}
                </td>
                <td className="py-2 text-right">
                  <button onClick={() => remove(a.id)} className="text-xs" style={{ color: 'var(--danger)' }}>
                    O'chirish
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <form onSubmit={create} className="flex flex-wrap gap-2 items-end">
        <div>
          <label className="block text-xs mb-1" style={{ color: 'var(--text-muted)' }}>
            Login
          </label>
          <input
            required
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            className="px-2 py-1.5 rounded"
            style={{ background: 'var(--surface-2)', border: '1px solid var(--border)' }}
          />
        </div>
        <div>
          <label className="block text-xs mb-1" style={{ color: 'var(--text-muted)' }}>
            Parol
          </label>
          <input
            required
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="px-2 py-1.5 rounded"
            style={{ background: 'var(--surface-2)', border: '1px solid var(--border)' }}
          />
        </div>
        <div>
          <label className="block text-xs mb-1" style={{ color: 'var(--text-muted)' }}>
            Rol
          </label>
          <select
            value={role}
            onChange={(e) => setRole(e.target.value as 'admin' | 'super_admin')}
            className="px-2 py-1.5 rounded"
            style={{ background: 'var(--surface-2)', border: '1px solid var(--border)' }}
          >
            <option value="admin">Admin</option>
            <option value="super_admin">Super-admin</option>
          </select>
        </div>
        <button type="submit" className="px-3 py-1.5 rounded text-white text-sm" style={{ background: 'var(--accent)' }}>
          Qo'shish
        </button>
      </form>
      {error && (
        <p className="text-sm mt-2" style={{ color: 'var(--danger)' }}>
          {error}
        </p>
      )}
    </div>
  )
}
