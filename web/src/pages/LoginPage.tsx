import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { ApiError } from '../api/client'

export default function LoginPage() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [totpCode, setTotpCode] = useState('')
  const [needsTotp, setNeedsTotp] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      await login(username, password, totpCode)
      navigate('/')
    } catch (err) {
      if (err instanceof ApiError && err.message.includes('2FA')) {
        setNeedsTotp(true)
        setError(err.message)
      } else {
        setError(err instanceof Error ? err.message : 'Kirishda xatolik yuz berdi')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center px-4" style={{ background: 'var(--bg)' }}>
      <form
        onSubmit={handleSubmit}
        className="w-full max-w-sm rounded-2xl p-8 shadow-xl"
        style={{ background: 'var(--surface)', border: '1px solid var(--border)' }}
      >
        <h1 className="text-xl font-semibold mb-1" style={{ color: 'var(--text)' }}>
          Boshqaruv paneli
        </h1>
        <p className="text-sm mb-6" style={{ color: 'var(--text-muted)' }}>
          Faqat adminlar uchun kirish
        </p>

        <label className="block text-sm mb-1" style={{ color: 'var(--text-muted)' }}>
          Login
        </label>
        <input
          className="w-full mb-4 px-3 py-2 rounded-lg outline-none"
          style={{ background: 'var(--surface-2)', border: '1px solid var(--border)', color: 'var(--text)' }}
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          autoFocus
          required
        />

        <label className="block text-sm mb-1" style={{ color: 'var(--text-muted)' }}>
          Parol
        </label>
        <input
          type="password"
          className="w-full mb-4 px-3 py-2 rounded-lg outline-none"
          style={{ background: 'var(--surface-2)', border: '1px solid var(--border)', color: 'var(--text)' }}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required
        />

        {needsTotp && (
          <>
            <label className="block text-sm mb-1" style={{ color: 'var(--text-muted)' }}>
              2FA kodi
            </label>
            <input
              className="w-full mb-4 px-3 py-2 rounded-lg outline-none tracking-widest"
              style={{ background: 'var(--surface-2)', border: '1px solid var(--border)', color: 'var(--text)' }}
              value={totpCode}
              onChange={(e) => setTotpCode(e.target.value)}
              maxLength={6}
              inputMode="numeric"
            />
          </>
        )}

        {error && (
          <p className="text-sm mb-4" style={{ color: 'var(--danger)' }}>
            {error}
          </p>
        )}

        <button
          type="submit"
          disabled={submitting}
          className="w-full py-2 rounded-lg font-medium text-white disabled:opacity-60"
          style={{ background: 'var(--accent)' }}
        >
          {submitting ? 'Kirilmoqda...' : 'Kirish'}
        </button>
      </form>
    </div>
  )
}
