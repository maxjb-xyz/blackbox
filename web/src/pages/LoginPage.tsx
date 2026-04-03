import { useEffect, useState } from 'react'
import { AlertCircle, Terminal } from 'lucide-react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { checkHealth, login } from '../api/client'

function sanitizeRedirectTo(value: string | null) {
  if (!value) return '/timeline'
  if (!value.startsWith('/')) return '/timeline'
  if (value.includes('://') || value.startsWith('//') || value.includes('..')) return '/timeline'
  return value
}

export default function LoginPage() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [oidcEnabled, setOidcEnabled] = useState(false)

  useEffect(() => {
    checkHealth()
      .then(health => setOidcEnabled(health.oidc_enabled))
      .catch(err => console.error('Health check failed', err))
  }, [])

  const redirectTo = sanitizeRedirectTo(searchParams.get('redirect_to'))

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setLoading(true)
    try {
      const token = await login(username, password)
      localStorage.setItem('token', token)
      navigate(sanitizeRedirectTo(redirectTo), { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed')
    } finally {
      setLoading(false)
    }
  }

  const inputStyle: React.CSSProperties = {
    width: '100%',
    background: 'var(--surface)',
    border: '1px solid var(--border)',
    color: 'var(--text)',
    padding: '8px 10px',
    fontFamily: 'inherit',
    fontSize: '13px',
  }

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'var(--bg)',
      }}
    >
      <div style={{ width: '100%', maxWidth: 320, border: '1px solid var(--border)', padding: '2rem' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 24 }}>
          <Terminal size={16} style={{ color: 'var(--accent)' }} />
          <span style={{ color: 'var(--accent)', fontSize: '11px', letterSpacing: '0.1em' }}>
            BLACKBOX / LOGIN
          </span>
        </div>

        <form onSubmit={handleSubmit}>
          <div style={{ marginBottom: 12 }}>
            <label
              htmlFor="username"
              style={{
                display: 'block',
                color: 'var(--muted)',
                fontSize: '11px',
                marginBottom: 4,
                letterSpacing: '0.05em',
              }}
            >
              USERNAME
            </label>
            <input
              id="username"
              className="login-input"
              type="text"
              value={username}
              onChange={e => setUsername(e.target.value)}
              required
              autoFocus
              style={inputStyle}
            />
          </div>

          <div style={{ marginBottom: 20 }}>
            <label
              htmlFor="password"
              style={{
                display: 'block',
                color: 'var(--muted)',
                fontSize: '11px',
                marginBottom: 4,
                letterSpacing: '0.05em',
              }}
            >
              PASSWORD
            </label>
            <input
              id="password"
              className="login-input"
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              required
              style={inputStyle}
            />
          </div>

          {error && (
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 6,
                color: 'var(--danger)',
                fontSize: '12px',
                marginBottom: 12,
              }}
            >
              <AlertCircle size={14} />
              <span>{error}</span>
            </div>
          )}

          <button
            type="submit"
            disabled={loading}
            style={{
              width: '100%',
              background: loading ? 'var(--border)' : 'var(--accent)',
              color: '#000',
              border: 'none',
              padding: '10px',
              fontFamily: 'inherit',
              fontSize: '12px',
              fontWeight: 'bold',
              letterSpacing: '0.1em',
              cursor: loading ? 'not-allowed' : 'pointer',
              marginBottom: oidcEnabled ? 8 : 0,
            }}
          >
            {loading ? 'LOGGING IN...' : 'LOGIN'}
          </button>

          {oidcEnabled && (
            <a
              href="/api/auth/oidc/login"
              style={{
                display: 'block',
                width: '100%',
                textAlign: 'center',
                background: 'transparent',
                border: '1px solid var(--border)',
                color: 'var(--muted)',
                padding: '10px',
                fontFamily: 'inherit',
                fontSize: '12px',
                letterSpacing: '0.1em',
                textDecoration: 'none',
                boxSizing: 'border-box',
              }}
            >
              LOGIN WITH SSO
            </a>
          )}
        </form>
      </div>
    </div>
  )
}
