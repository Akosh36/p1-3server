import { Navigate, Route, Routes } from 'react-router-dom'
import { useAuth } from './context/AuthContext'
import Layout from './components/Layout'
import LoginPage from './pages/LoginPage'
import DashboardPage from './pages/DashboardPage'
import ServerPage from './pages/ServerPage'
import UsersPage from './pages/UsersPage'
import AdminsPage from './pages/AdminsPage'
import ServersPage from './pages/ServersPage'
import LANPage from './pages/LANPage'
import FirewallPage from './pages/FirewallPage'
import LogsPage from './pages/LogsPage'

function RequireAuth({ children }: { children: React.ReactNode }) {
  const { admin, loading } = useAuth()
  if (loading) return null
  if (!admin) return <Navigate to="/login" replace />
  return <>{children}</>
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/"
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route index element={<DashboardPage />} />
        <Route path="server" element={<ServerPage />} />
        <Route path="users" element={<UsersPage />} />
        <Route path="admins" element={<AdminsPage />} />
        <Route path="servers" element={<ServersPage />} />
        <Route path="lan" element={<LANPage />} />
        <Route path="firewall" element={<FirewallPage />} />
        <Route path="logs" element={<LogsPage />} />
      </Route>
    </Routes>
  )
}
