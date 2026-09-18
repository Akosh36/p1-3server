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
  const [allowedIp, setAllowedIp] = useState('')
  const [allowedMac, setAllowedMac] = useState('')
  const [error, setError] = useState<string | null>(null)

  async function create(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await api.post('/admins', {
        username,
        password,
        role,
        allowed_ip: allowedIp || undefined,
        allowed_mac: allowedMac || undefined,
      })
      setUsername('')
      setPassword('')
      setAllowedIp('')
      setAllowedMac('')
      onChanged()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create admin')
    }
  }

  async function toggleActive(a: Admin) {
    if (a.is_active && !confirm(`"${a.username}" hisobini faolsizlantirasizmi? U darhol tizimdan chiqariladi.`)) return
    try {
      await api.patch(`/admins/${a.id}`, { is_active: !a.is_active })
      onChanged()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update admin')
    }
  }

  async function remove(id: number) {
    if (!confirm("Bu panel hisobini butunlay o'chirasizmi? (Faolsizlantirish tavsiya etiladi — tarix saqlanadi.)")) return
    try {
      await api.delete(`/admins/${id}`)
      onChanged()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete admin')
    }
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
              <th className="pb-2 font-medium">IP/MAC cheklovi</th>
              <th className="pb-2 font-medium">Holat</th>
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
                <td className="py-2 text-xs" style={{ color: 'var(--text-muted)' }}>
                  {a.allowed_ip || a.allowed_mac ? (
                    <>
                      {a.allowed_ip && <div>IP: {a.allowed_ip}</div>}
                      {a.allowed_mac && <div>MAC: {a.allowed_mac}</div>}
                    </>
                  ) : (
                    '— (cheklanmagan)'
                  )}
                </td>
                <td className="py-2">
                  <button
                    onClick={() => toggleActive(a)}
                    className="px-2 py-0.5 rounded-full text-xs font-medium"
                    style={{
                      background: a.is_active ? 'color-mix(in srgb, var(--success) 15%, transparent)' : 'color-mix(in srgb, var(--text-muted) 15%, transparent)',
                      color: a.is_active ? 'var(--success)' : 'var(--text-muted)',
                    }}
                  >
                    {a.is_active ? 'Faol' : 'Faolsizlantirilgan'}
                  </button>
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
            Parol (kamida 8 belgi)
          </label>
          <input
            required
            minLength={8}
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
        <div>
          <label className="block text-xs mb-1" style={{ color: 'var(--text-muted)' }}>
            Ruxsat etilgan IP (ixtiyoriy)
          </label>
          <input
            value={allowedIp}
            onChange={(e) => setAllowedIp(e.target.value)}
            placeholder="192.168.1.5 yoki 192.168.1.0/24"
            className="px-2 py-1.5 rounded"
            style={{ background: 'var(--surface-2)', border: '1px solid var(--border)' }}
          />
        </div>
        <div>
          <label className="block text-xs mb-1" style={{ color: 'var(--text-muted)' }}>
            Ruxsat etilgan MAC (ixtiyoriy)
          </label>
          <input
            value={allowedMac}
            onChange={(e) => setAllowedMac(e.target.value)}
            placeholder="aa:bb:cc:dd:ee:ff"
            className="px-2 py-1.5 rounded font-mono text-xs"
            style={{ background: 'var(--surface-2)', border: '1px solid var(--border)' }}
          />
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
