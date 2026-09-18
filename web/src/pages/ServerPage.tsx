import { CartesianGrid, Legend, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { usePolling } from '../hooks/usePolling'
import { api } from '../api/client'
import type { SystemMetric } from '../api/types'
import { Panel, StatTile, EmptyNote } from '../components/Card'
import { CHART_CHROME, CHART_COLORS } from '../theme/palette'

function fmtTime(iso: string) {
  return new Date(iso).toLocaleTimeString('uz-UZ', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function fmtBytes(v: number) {
  if (v > 1024 * 1024) return `${(v / (1024 * 1024)).toFixed(1)} MB/s`
  if (v > 1024) return `${(v / 1024).toFixed(1)} KB/s`
  return `${v} B/s`
}

export default function ServerPage() {
  const { data: latest } = usePolling(() => api.get<SystemMetric | null>('/metrics/self'), 5000)
  const { data: history } = usePolling(() => api.get<SystemMetric[]>('/metrics/history?limit=120'), 10000)

  const chartData = (history ?? [])
    .slice()
    .reverse()
    .map((m) => ({ ...m, timeLabel: fmtTime(m.time) }))

  return (
    <div>
      <h1 className="text-xl font-semibold mb-4" style={{ color: 'var(--text)' }}>
        Server (Load Balancer / Gateway hosti)
      </h1>

      <div className="grid grid-cols-2 md:grid-cols-5 gap-3 mb-6">
        <StatTile label="CPU" value={latest ? `${latest.cpu_percent.toFixed(1)}%` : '—'} />
        <StatTile label="RAM" value={latest ? `${latest.mem_percent.toFixed(1)}%` : '—'} />
        <StatTile label="Disk" value={latest ? `${latest.disk_percent.toFixed(1)}%` : '—'} />
        <StatTile
          label="Tarmoq"
          value={latest ? `${fmtBytes(latest.net_in_bps)} ↓` : '—'}
          hint={latest ? `${fmtBytes(latest.net_out_bps)} ↑` : undefined}
        />
        <StatTile
          label="GPU"
          value={latest?.gpu_percent != null ? `${latest.gpu_percent.toFixed(1)}%` : 'Mavjud emas'}
          hint={latest?.gpu_mem_percent != null ? `Xotira: ${latest.gpu_mem_percent.toFixed(1)}%` : undefined}
        />
      </div>

      <Panel title="CPU / RAM / Disk (%)">
        {chartData.length < 2 ? (
          <EmptyNote>Grafik uchun kamida bir necha o'lchov kerak — bir necha soniyadan so'ng to'ladi.</EmptyNote>
        ) : (
          <ResponsiveContainer width="100%" height={260}>
            <LineChart data={chartData} margin={{ left: 0, right: 8 }}>
              <CartesianGrid stroke={CHART_CHROME.grid} vertical={false} />
              <XAxis dataKey="timeLabel" stroke={CHART_CHROME.axis} tick={{ fill: CHART_CHROME.mutedText, fontSize: 12 }} minTickGap={40} />
              <YAxis domain={[0, 100]} stroke={CHART_CHROME.axis} tick={{ fill: CHART_CHROME.mutedText, fontSize: 12 }} width={36} />
              <Tooltip />
              <Legend />
              <Line type="monotone" dataKey="cpu_percent" name="CPU %" stroke={CHART_COLORS.slot1Blue} strokeWidth={2} dot={false} />
              <Line type="monotone" dataKey="mem_percent" name="RAM %" stroke={CHART_COLORS.slot2Orange} strokeWidth={2} dot={false} />
              <Line type="monotone" dataKey="disk_percent" name="Disk %" stroke={CHART_COLORS.slot3Aqua} strokeWidth={2} dot={false} />
            </LineChart>
          </ResponsiveContainer>
        )}
      </Panel>

      <Panel title="Tarmoq tezligi (B/s)">
        {chartData.length < 2 ? (
          <EmptyNote>Ma'lumot yig'ilmoqda...</EmptyNote>
        ) : (
          <ResponsiveContainer width="100%" height={220}>
            <LineChart data={chartData} margin={{ left: 0, right: 8 }}>
              <CartesianGrid stroke={CHART_CHROME.grid} vertical={false} />
              <XAxis dataKey="timeLabel" stroke={CHART_CHROME.axis} tick={{ fill: CHART_CHROME.mutedText, fontSize: 12 }} minTickGap={40} />
              <YAxis stroke={CHART_CHROME.axis} tick={{ fill: CHART_CHROME.mutedText, fontSize: 12 }} width={56} tickFormatter={fmtBytes} />
              <Tooltip formatter={(v) => fmtBytes(Number(v))} />
              <Legend />
              <Line type="monotone" dataKey="net_in_bps" name="Kirish" stroke={CHART_COLORS.slot1Blue} strokeWidth={2} dot={false} />
              <Line type="monotone" dataKey="net_out_bps" name="Chiqish" stroke={CHART_COLORS.slot2Orange} strokeWidth={2} dot={false} />
            </LineChart>
          </ResponsiveContainer>
        )}
      </Panel>

      <Panel title="Disk I/O (B/s)">
        {chartData.length < 2 ? (
          <EmptyNote>Ma'lumot yig'ilmoqda...</EmptyNote>
        ) : (
          <ResponsiveContainer width="100%" height={220}>
            <LineChart data={chartData} margin={{ left: 0, right: 8 }}>
              <CartesianGrid stroke={CHART_CHROME.grid} vertical={false} />
              <XAxis dataKey="timeLabel" stroke={CHART_CHROME.axis} tick={{ fill: CHART_CHROME.mutedText, fontSize: 12 }} minTickGap={40} />
              <YAxis stroke={CHART_CHROME.axis} tick={{ fill: CHART_CHROME.mutedText, fontSize: 12 }} width={56} tickFormatter={fmtBytes} />
              <Tooltip formatter={(v) => fmtBytes(Number(v))} />
              <Legend />
              <Line type="monotone" dataKey="disk_read_bps" name="O'qish" stroke={CHART_COLORS.slot1Blue} strokeWidth={2} dot={false} />
              <Line type="monotone" dataKey="disk_write_bps" name="Yozish" stroke={CHART_COLORS.slot2Orange} strokeWidth={2} dot={false} />
            </LineChart>
          </ResponsiveContainer>
        )}
      </Panel>
    </div>
  )
}
