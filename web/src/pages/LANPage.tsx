import { usePolling } from '../hooks/usePolling'
import { api } from '../api/client'
import type { LANNetwork, SwitchPort } from '../api/types'
import { Panel, EmptyNote } from '../components/Card'

export default function LANPage() {
  const { data: ports } = usePolling(() => api.get<SwitchPort[]>('/switch-ports'))
  const { data: networks, refresh: refreshNetworks } = usePolling(() => api.get<LANNetwork[]>('/lan-networks'))

  async function toggleNetwork(n: LANNetwork) {
    await api.patch(`/lan-networks/${n.id}`, { is_active: !n.is_active })
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

      <Panel title="Wireless (lokal WiFi + masofaviy VPN LAN)">
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
              </tr>
            </thead>
            <tbody>
              {(networks ?? []).map((n) => (
                <tr key={n.id} style={{ borderTop: '1px solid var(--border)' }}>
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
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Panel>
    </div>
  )
}
