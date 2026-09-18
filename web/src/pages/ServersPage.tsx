import { useState, type FormEvent } from 'react'
import { usePolling } from '../hooks/usePolling'
import { api } from '../api/client'
import type { ServerGroup } from '../api/types'
import { Panel, EmptyNote } from '../components/Card'

export default function ServersPage() {
  const { data: groups, refresh } = usePolling(() => api.get<ServerGroup[]>('/server-groups'))
  const [showCreate, setShowCreate] = useState(false)

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-xl font-semibold" style={{ color: 'var(--text)' }}>
          Serverlar (Load Balancing guruhlari)
        </h1>
        <button
          onClick={() => setShowCreate((v) => !v)}
          className="px-3 py-1.5 rounded text-white text-sm"
          style={{ background: 'var(--accent)' }}
        >
          + Yangi guruh
        </button>
      </div>

      {showCreate && (
        <Panel>
          <CreateGroupForm
            onCreated={() => {
              setShowCreate(false)
              refresh()
            }}
          />
        </Panel>
      )}

      {(groups ?? []).length === 0 && <EmptyNote>Hali server guruhi yaratilmagan.</EmptyNote>}

      {(groups ?? []).map((g) => (
        <GroupCard key={g.id} group={g} onChanged={refresh} />
      ))}
    </div>
  )
}

function CreateGroupForm({ onCreated }: { onCreated: () => void }) {
  const [nickname, setNickname] = useState('')
  const [vip, setVip] = useState('')
  const [port, setPort] = useState('80')
  const [color, setColor] = useState('#4f46e5')
  const [error, setError] = useState<string | null>(null)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await api.post('/server-groups', {
        nickname,
        vip_address: vip,
        vip_port: Number(port),
        color_hex: color,
        algorithm: 'round_robin',
      })
      onCreated()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create group')
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-wrap gap-3 items-end">
      <Field label="Nickname">
        <input required value={nickname} onChange={(e) => setNickname(e.target.value)} className="input" />
      </Field>
      <Field label="VIP manzil">
        <input required placeholder="192.168.1.10" value={vip} onChange={(e) => setVip(e.target.value)} className="input" />
      </Field>
      <Field label="Port">
        <input required type="number" value={port} onChange={(e) => setPort(e.target.value)} className="input w-24" />
      </Field>
      <Field label="Rang">
        <input type="color" value={color} onChange={(e) => setColor(e.target.value)} className="w-10 h-9 rounded" />
      </Field>
      <button type="submit" className="px-3 py-1.5 rounded text-white text-sm" style={{ background: 'var(--accent)' }}>
        Yaratish
      </button>
      {error && (
        <p className="text-sm" style={{ color: 'var(--danger)' }}>
          {error}
        </p>
      )}
    </form>
  )
}

// lbd (Phase 4) writes real last_check_at/is_healthy back via lbsync, so
// "never checked" (no lbd sync has happened yet, e.g. right after adding a
// backend or while lbd is down) and "checked and currently failing" are
// distinguishable states, not both just "false" — showing them the same
// would make a real outage look identical to a lbd that hasn't run yet.
function BackendHealthBadge({ backend }: { backend: import('../api/types').BackendServer }) {
  if (!backend.last_check_at) {
    return <span style={{ color: 'var(--text-muted)' }}>○ Tekshirilmagan</span>
  }
  if (backend.is_healthy) {
    return <span style={{ color: 'var(--success)' }}>● Sog'lom</span>
  }
  return <span style={{ color: 'var(--danger)' }}>● Ishlamayapti</span>
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="block text-xs mb-1" style={{ color: 'var(--text-muted)' }}>
        {label}
      </label>
      {children}
    </div>
  )
}

function GroupCard({ group, onChanged }: { group: ServerGroup; onChanged: () => void }) {
  const [ip, setIp] = useState('')
  const [port, setPort] = useState('')

  async function addBackend(e: FormEvent) {
    e.preventDefault()
    await api.post(`/server-groups/${group.id}/backends`, { ip, port: Number(port), weight: 1 })
    setIp('')
    setPort('')
    onChanged()
  }

  async function removeBackend(id: number) {
    await api.delete(`/backends/${id}`)
    onChanged()
  }

  async function removeGroup() {
    if (!confirm(`"${group.nickname}" guruhini o'chirasizmi?`)) return
    await api.delete(`/server-groups/${group.id}`)
    onChanged()
  }

  return (
    <Panel
      title={
        <span className="flex items-center gap-2">
          <span className="w-3 h-3 rounded-full inline-block" style={{ background: group.color_hex }} />
          {group.nickname}
          <span className="text-xs font-normal" style={{ color: 'var(--text-muted)' }}>
            {group.vip_address}:{group.vip_port} · {group.algorithm}
          </span>
        </span>
      }
      action={
        <button onClick={removeGroup} className="text-xs" style={{ color: 'var(--danger)' }}>
          Guruhni o'chirish
        </button>
      }
    >
      {group.backends.length === 0 ? (
        <EmptyNote>Bu guruhda hali backend server yo'q.</EmptyNote>
      ) : (
        <table className="w-full text-sm mb-3">
          <thead>
            <tr style={{ color: 'var(--text-muted)' }} className="text-left">
              <th className="pb-2 font-medium">IP:Port</th>
              <th className="pb-2 font-medium">Weight</th>
              <th className="pb-2 font-medium">Holat</th>
              <th className="pb-2 font-medium">Javob vaqti</th>
              <th className="pb-2"></th>
            </tr>
          </thead>
          <tbody>
            {group.backends.map((b) => (
              <tr key={b.id} style={{ borderTop: '1px solid var(--border)' }}>
                <td className="py-2" style={{ color: 'var(--text)' }}>
                  {b.ip}:{b.port}
                </td>
                <td className="py-2" style={{ color: 'var(--text)' }}>
                  {b.weight}
                </td>
                <td className="py-2">
                  <BackendHealthBadge backend={b} />
                </td>
                <td className="py-2" style={{ color: 'var(--text-muted)' }}>
                  {b.last_check_at && b.response_time_ms != null ? `${b.response_time_ms.toFixed(1)} ms` : '—'}
                </td>
                <td className="py-2 text-right">
                  <button onClick={() => removeBackend(b.id)} className="text-xs" style={{ color: 'var(--danger)' }}>
                    O'chirish
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <form onSubmit={addBackend} className="flex gap-2 items-end">
        <Field label="Backend IP">
          <input required value={ip} onChange={(e) => setIp(e.target.value)} className="input" placeholder="10.0.0.5" />
        </Field>
        <Field label="Port">
          <input required type="number" value={port} onChange={(e) => setPort(e.target.value)} className="input w-24" />
        </Field>
        <button type="submit" className="px-3 py-1.5 rounded text-sm" style={{ border: '1px solid var(--border)', color: 'var(--text)' }}>
          + Backend qo'shish
        </button>
      </form>
    </Panel>
  )
}
