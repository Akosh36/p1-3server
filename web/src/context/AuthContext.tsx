import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { api, clearToken, getToken, setToken } from '../api/client'
import type { Admin } from '../api/types'

interface AuthState {
  admin: Admin | null
  loading: boolean
  login: (username: string, password: string, totpCode?: string) => Promise<void>
  logout: () => void
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [admin, setAdmin] = useState<Admin | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!getToken()) {
      setLoading(false)
      return
    }
    api
      .get<Admin>('/auth/me')
      .then(setAdmin)
      .catch(() => clearToken())
      .finally(() => setLoading(false))
  }, [])

  async function login(username: string, password: string, totpCode?: string) {
    const res = await api.post<{ token: string; admin: Admin }>('/auth/login', {
      username,
      password,
      totp_code: totpCode || undefined,
    })
    setToken(res.token)
    setAdmin(res.admin)
  }

  function logout() {
    clearToken()
    setAdmin(null)
  }

  return <AuthContext.Provider value={{ admin, loading, login, logout }}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
