import { usePolling } from '../hooks/usePolling'
import { api } from '../api/client'
import type { Device } from '../api/types'
import { Panel } from '../components/Card'
import DeviceTable from '../components/DeviceTable'

export default function UsersPage() {
  const { data: devices, refresh } = usePolling(() => api.get<Device[]>('/devices'))

  return (
    <div>
      <h1 className="text-xl font-semibold mb-4" style={{ color: 'var(--text)' }}>
        Userlar
      </h1>
      <Panel title="'User' huquqiga ega qurilmalar">
        <DeviceTable devices={devices ?? []} role="user" showCapture onChanged={refresh} />
      </Panel>
    </div>
  )
}
