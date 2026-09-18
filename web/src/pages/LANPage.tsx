import { Fragment, useState, type FormEvent } from 'react'
import { usePolling } from '../hooks/usePolling'
import { api } from '../api/client'
import type { Device, LANNetwork, SwitchPort, WireGuardLocalInfo } from '../api/types'
import { Panel, EmptyNote } from '../components/Card'
import AllDevicesTable from '../components/AllDevicesTable'

function fmtHandshake(iso?: string) {
  if (!iso) return 'Hech qachon'
  return new Date(iso).toLocaleString('uz-UZ')
}

function LocalWireGuardInfo() {
  const { data } = usePolling(() => api.get<WireGuardLocalInfo>('/wireguard/local-info'), 15000)
  const [copied, setCopied] = useState(false)

  async function copy() {
    if (!data?.public_key) return
    try {
      await navigator.clipboard.writeText(data.public_key)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // clipboard API unavailable — key is still visible/selectable below
    }
  }

  if (!data) {
    return (
      <EmptyNote>
        wgd hali javob bermayapti — u ishga tushgach, shu yerda bu serverning WireGuard public key va porti ko'rinadi
        (masofaviy tomon administratoriga shuni berish kerak bo'ladi).
      </EmptyNote>
    )
  }

  return (
    <div className="flex items-center gap-2 flex-wrap text-sm">
      <span style={{ color: 'var(--text-muted)' }}>Bu serverning public key'i:</span>
      <code className="text-xs px-2 py-1 rounded select-all" style={{ background: 'var(--surface-2)', color: 'var(--text)' }}>
        {data.public_key}
      </code>
      <button onClick={copy} className="text-xs" style={{ color: 'var(--accent)' }}>
        {copied ? 'Nusxalandi ✓' : 'Nusxalash'}
      </button>
      <span style={{ color: 'var(--text-muted)' }}>· Port: {data.listen_port}</span>
    </div>
  )
}

function CreateRemoteVPNForm({ onCreated }: { onCreated: () => void }) {
  const [name, setName] = useState('')
  const [publicKey, setPublicKey] = useState('')
  const [allowedSubnet, setAllowedSubnet] = useState('')
  const [endpoint, setEndpoint] = useState('')
  const [error, setError] = useState<string | null>(null)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await api.post('/lan-networks/remote-vpn', {
        name,
        public_key: publicKey,
        allowed_subnet: allowedSubnet,
        endpoint: endpoint || undefined,
      })
      setName('')
      setPublicKey('')
      setAllowedSubnet('')
      setEndpoint('')
      onCreated()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create')
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-wrap gap-3 items-end">
      <div>
        <label className="block text-xs mb-1" style={{ color: 'var(--text-muted)' }}>
          Nomi
        </label>
        <input required value={name} onChange={(e) => setName(e.target.value)} className="input" placeholder="Filial-2 ofisi" />
      </div>
      <div>
        <label className="block text-xs mb-1" style={{ color: 'var(--text-muted)' }}>
          Masofaviy tomonning public key'i
        </label>
        <input
          required
          value={publicKey}
          onChange={(e) => setPublicKey(e.target.value)}
          className="input font-mono text-xs"
          style={{ width: '20rem' }}
          placeholder="wg genkey | wg pubkey natijasi"
        />
      </div>
      <div>
        <label className="block text-xs mb-1" style={{ color: 'var(--text-muted)' }}>
          Masofaviy LAN subneti (CIDR)
        </label>
        <input
          required
          value={allowedSubnet}
          onChange={(e) => setAllowedSubnet(e.target.value)}
          className="input"
          placeholder="192.168.50.0/24"
        />
      </div>
      <div>
        <label className="block text-xs mb-1" style={{ color: 'var(--text-muted)' }}>
          Endpoint (ixtiyoriy)
        </label>
        <input value={endpoint} onChange={(e) => setEndpoint(e.target.value)} className="input" placeholder="203.0.113.5:51820" />
      </div>
      <button type="submit" className="px-3 py-1.5 rounded text-white text-sm" style={{ background: 'var(--accent)' }}>
        Qo'shish
      </button>
      {error && (
        <p className="text-sm w-full" style={{ color: 'var(--danger)' }}>
          {error}
        </p>
      )}
    </form>
  )
}

export default function LANPage() {
  const { data: ports } = usePolling(() => api.get<SwitchPort[]>('/switch-ports'))
  const { data: networks, refresh: refreshNetworks } = usePolling(() => api.get<LANNetwork[]>('/lan-networks'))
  const { data: devices, refresh: refreshDevices } = usePolling(() => api.get<Device[]>('/devices'))
  const [showCreate, setShowCreate] = useState(false)
  const [expandedId, setExpandedId] = useState<number | null>(null)

  async function toggleNetwork(n: LANNetwork) {
    await api.patch(`/lan-networks/${n.id}`, { is_active: !n.is_active })
    refreshNetworks()
  }

  async function deleteNetwork(n: LANNetwork) {
    if (!confirm(`"${n.name}" tarmog'ini o'chirasizmi?`)) return
    await api.delete(`/lan-networks/${n.id}`)
    refreshNetworks()
  }

  return (
    <div>
      <h1 className="text-xl font-semibold mb-4" style={{ color: 'var(--text)' }}>
        LAN
      </h1>

      <Panel title="Portlar (kabelli, boshqariladigan switch)">
        {(ports ?? []).length === 0 ? (
          <EmptyNote>
            Hali port ma'lumoti yo'q — netdiscd daemoni (Phase 1) switch'ni SNMP orqali so'rab, bu jadvalni to'ldiradi.
          </EmptyNote>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr style={{ color: 'var(--text-muted)' }} className="text-left">
                <th className="pb-2 font-medium">Switch</th>
                <th className="pb-2 font-medium">Port</th>
                <th className="pb-2 font-medium">Label</th>
                <th className="pb-2 font-medium">VLAN</th>
                <th className="pb-2 font-medium">Holat</th>
              </tr>
            </thead>
            <tbody>
              {(ports ?? []).map((p) => (
                <tr key={p.id} style={{ borderTop: '1px solid var(--border)' }}>
                  <td className="py-2" style={{ color: 'var(--text)' }}>{p.switch_name}</td>
                  <td className="py-2" style={{ color: 'var(--text)' }}>{p.port_number}</td>
                  <td className="py-2" style={{ color: 'var(--text)' }}>{p.label || '—'}</td>
                  <td className="py-2" style={{ color: 'var(--text)' }}>{p.vlan ?? '—'}</td>
                  <td className="py-2">
                    <span style={{ color: p.link_status ? 'var(--success)' : 'var(--text-muted)' }}>
                      {p.link_status ? '● Ulangan' : '○ Uzilgan'}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Panel>

      <Panel
        title="Wireless (lokal WiFi + masofaviy VPN LAN)"
        action={
          <button
            onClick={() => setShowCreate((v) => !v)}
            className="px-3 py-1.5 rounded text-white text-sm"
            style={{ background: 'var(--accent)' }}
          >
            + Masofaviy LAN qo'shish
          </button>
        }
      >
        <div className="mb-4">
          <LocalWireGuardInfo />
        </div>

        {showCreate && (
          <div className="mb-4 pb-4" style={{ borderBottom: '1px solid var(--border)' }}>
            <CreateRemoteVPNForm
              onCreated={() => {
                setShowCreate(false)
                refreshNetworks()
              }}
            />
          </div>
        )}

        {(networks ?? []).length === 0 ? (
          <EmptyNote>Hali wireless/LAN tarmog'i ro'yxatga olinmagan.</EmptyNote>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr style={{ color: 'var(--text-muted)' }} className="text-left">
                <th className="pb-2 font-medium">Nomi</th>
                <th className="pb-2 font-medium">Turi</th>
                <th className="pb-2 font-medium">Reachable</th>
                <th className="pb-2 font-medium">Faol</th>
                <th className="pb-2"></th>
              </tr>
            </thead>
            <tbody>
              {(networks ?? []).map((n) => (
                <Fragment key={n.id}>
                  <tr style={{ borderTop: '1px solid var(--border)' }}>
                    <td className="py-2" style={{ color: 'var(--text)' }}>{n.name}</td>
                    <td className="py-2" style={{ color: 'var(--text)' }}>
                      {n.type === 'wireless_remote_vpn' ? 'Masofaviy (VPN)' : 'Lokal WiFi'}
                    </td>
                    <td className="py-2">
                      <span style={{ color: n.is_reachable ? 'var(--success)' : 'var(--danger)' }}>
                        {n.is_reachable ? '● Bor' : '○ Yo\'q'}
                      </span>
                    </td>
                    <td className="py-2">
                      <button
                        onClick={() => toggleNetwork(n)}
                        className="px-2 py-1 rounded text-xs"
                        style={{
                          background: n.is_active ? 'color-mix(in srgb, var(--success) 15%, transparent)' : 'color-mix(in srgb, var(--text-muted) 15%, transparent)',
                          color: n.is_active ? 'var(--success)' : 'var(--text-muted)',
                        }}
                      >
                        {n.is_active ? 'Faol (bosib o\'chirish)' : 'O\'chirilgan (bosib yoqish)'}
                      </button>
                    </td>
                    <td className="py-2 text-right whitespace-nowrap">
                      {n.type === 'wireless_remote_vpn' && (
                        <button
                          onClick={() => setExpandedId(expandedId === n.id ? null : n.id)}
                          className="text-xs mr-3"
                          style={{ color: 'var(--accent)' }}
                        >
                          {expandedId === n.id ? 'Yopish' : 'Tafsilot'}
                        </button>
                      )}
                      <button onClick={() => deleteNetwork(n)} className="text-xs" style={{ color: 'var(--danger)' }}>
                        O'chirish
                      </button>
                    </td>
                  </tr>
                  {expandedId === n.id && n.type === 'wireless_remote_vpn' && (
                    <tr style={{ borderTop: '1px solid var(--border)' }}>
                      <td colSpan={5} className="py-3">
                        <div className="grid grid-cols-2 gap-2 text-xs" style={{ color: 'var(--text-muted)' }}>
                          <div>
                            Masofaviy public key:{' '}
                            <code className="select-all" style={{ color: 'var(--text)' }}>
                              {n.vpn_public_key}
                            </code>
                          </div>
                          <div>
                            Masofaviy LAN subneti: <span style={{ color: 'var(--text)' }}>{n.vpn_allowed_subnet}</span>
                          </div>
                          <div>
                            Endpoint: <span style={{ color: 'var(--text)' }}>{n.vpn_endpoint || '—'}</span>
                          </div>
                          <div>
                            So'nggi handshake: <span style={{ color: 'var(--text)' }}>{fmtHandshake(n.vpn_last_handshake_at)}</span>
                          </div>
                        </div>
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
        )}
      </Panel>

      <Panel title="Barcha aniqlangan qurilmalar — nom berish va huquq belgilash">
        <AllDevicesTable devices={devices ?? []} onChanged={refreshDevices} />
      </Panel>
    </div>
  )
}
