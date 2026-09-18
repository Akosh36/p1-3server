import { Fragment, useState, type FormEvent } from 'react'
import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { usePolling } from '../hooks/usePolling'
import { api } from '../api/client'
import type { BackendMetric, BackendServer, ServerGroup } from '../api/types'
import { Panel, EmptyNote } from '../components/Card'
import { CHART_CHROME, CHART_COLORS } from '../theme/palette'

function fmtTime(iso: string) {
  return new Date(iso).toLocaleTimeString('uz-UZ', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

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
function BackendHealthBadge({ backend }: { backend: BackendServer }) {
  if (!backend.last_check_at) {
    return <span style={{ color: 'var(--text-muted)' }}>○ Tekshirilmagan</span>
  }
  if (backend.is_healthy) {
    return <span style={{ color: 'var(--success)' }}>● Sog'lom</span>
  }
  return <span style={{ color: 'var(--danger)' }}>● Ishlamayapti</span>
}

// Phase 5: cmd/backendagentd pushes real host metrics from the backend
// server itself (see internal/httpapi's handleAgentMetrics), independent of
// lbd's own TCP-connect health check above — a backend can be "Sog'lom" from
// the LB's point of view while its host metrics are unavailable simply
// because the agent isn't installed there yet, so an empty chart is a
// normal, expected state, not an error.
function BackendDetail({ backend, onTokenChanged }: { backend: BackendServer; onTokenChanged: () => void }) {
  const { data: history } = usePolling(() => api.get<BackendMetric[]>(`/backends/${backend.id}/metrics?limit=60`), 10000)
  const [copied, setCopied] = useState(false)
  const [regenerating, setRegenerating] = useState(false)

  const chartData = (history ?? [])
    .slice()
    .reverse()
    .map((m) => ({ ...m, timeLabel: fmtTime(m.time) }))

  async function copyToken() {
    if (!backend.agent_token) return
    try {
      await navigator.clipboard.writeText(backend.agent_token)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // clipboard API unavailable (e.g. insecure context) — token is still
      // visible and selectable in the <code> block below.
    }
  }

  async function regenerateToken() {
    if (!confirm('Eski token ishlamay qoladi va agentni qayta sozlash kerak bo\'ladi. Davom etasizmi?')) return
    setRegenerating(true)
    try {
      await api.post(`/backends/${backend.id}/regenerate-token`)
      onTokenChanged()
    } finally {
      setRegenerating(false)
    }
  }

  return (
    <tr style={{ borderTop: '1px solid var(--border)' }}>
      <td colSpan={5} className="py-3">
        <div className="mb-3 flex items-center gap-2 flex-wrap">
          <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
            Agent token (cmd/backendagentd uchun AGENT_TOKEN):
          </span>
          <code
            className="text-xs px-2 py-1 rounded select-all"
            style={{ background: 'var(--bg-subtle, rgba(127,127,127,0.12))', color: 'var(--text)' }}
          >
            {backend.agent_token ?? '(yo\'q — pastdagi tugma bilan yarating)'}
          </code>
          {backend.agent_token && (
            <button onClick={copyToken} className="text-xs" style={{ color: 'var(--accent)' }}>
              {copied ? 'Nusxalandi ✓' : 'Nusxalash'}
            </button>
          )}
          <button onClick={regenerateToken} disabled={regenerating} className="text-xs" style={{ color: 'var(--text-muted)' }}>
            {regenerating ? 'Yaratilmoqda...' : 'Tokenni yangilash'}
          </button>
        </div>

        {chartData.length < 2 ? (
          <EmptyNote>
            Bu backend'dan hali metrikalar kelmagan — <code>cmd/backendagentd</code>ni shu serverda AGENT_API_URL va
            yuqoridagi AGENT_TOKEN bilan ishga tushiring.
          </EmptyNote>
        ) : (
          <ResponsiveContainer width="100%" height={160}>
            <LineChart data={chartData} margin={{ left: 0, right: 8 }}>
              <CartesianGrid stroke={CHART_CHROME.grid} vertical={false} />
              <XAxis dataKey="timeLabel" stroke={CHART_CHROME.axis} tick={{ fill: CHART_CHROME.mutedText, fontSize: 11 }} minTickGap={40} />
              <YAxis domain={[0, 100]} stroke={CHART_CHROME.axis} tick={{ fill: CHART_CHROME.mutedText, fontSize: 11 }} width={32} />
              <Tooltip />
              <Line type="monotone" dataKey="cpu_percent" name="CPU %" stroke={CHART_COLORS.slot1Blue} strokeWidth={2} dot={false} />
              <Line type="monotone" dataKey="mem_percent" name="RAM %" stroke={CHART_COLORS.slot2Orange} strokeWidth={2} dot={false} />
              <Line type="monotone" dataKey="disk_percent" name="Disk %" stroke={CHART_COLORS.slot3Aqua} strokeWidth={2} dot={false} />
            </LineChart>
          </ResponsiveContainer>
        )}
      </td>
    </tr>
  )
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
  const [expandedId, setExpandedId] = useState<number | null>(null)

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
              <Fragment key={b.id}>
                <tr style={{ borderTop: '1px solid var(--border)' }}>
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
                  <td className="py-2 text-right whitespace-nowrap">
                    <button
                      onClick={() => setExpandedId(expandedId === b.id ? null : b.id)}
                      className="text-xs mr-3"
                      style={{ color: 'var(--accent)' }}
                    >
                      {expandedId === b.id ? 'Yopish' : 'Metrikalar'}
                    </button>
                    <button onClick={() => removeBackend(b.id)} className="text-xs" style={{ color: 'var(--danger)' }}>
                      O'chirish
                    </button>
                  </td>
                </tr>
                {expandedId === b.id && <BackendDetail backend={b} onTokenChanged={onChanged} />}
              </Fragment>
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
