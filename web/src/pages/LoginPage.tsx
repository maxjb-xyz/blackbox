import { useEffect, useState } from 'react'
import { AlertCircle, Terminal } from 'lucide-react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { fetchOIDCProviders, login, type PublicOIDCProvider } from '../api/client'
import { useSession } from '../session'

function sanitizeRedirectTo(value: string | null) {
  if (!value) return '/timeline'
  if (!value.startsWith('/')) return '/timeline'
  if (value.includes('://') || value.startsWith('//') || value.includes('..')) return '/timeline'
  return value
}

export default function LoginPage() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { refreshSession } = useSession()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [oidcProviders, setOidcProviders] = useState<PublicOIDCProvider[]>([])
  const [oidcError, setOidcError] = useState<string | null>(null)

  useEffect(() => {
    setOidcError(null)
    fetchOIDCProviders()
      .then(data => setOidcProviders(data.providers))
      .catch(err => {
        console.error('OIDC provider fetch failed', err)
        setOidcProviders([])
        setOidcError(err instanceof Error ? err.message : 'Single sign-on is temporarily unavailable.')
      })
  }, [])

  const redirectTo = sanitizeRedirectTo(searchParams.get('redirect_to'))

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setLoading(true)
    try {
      await login(username, password)
      let user = null
      try {
        user = await refreshSession()
      } catch (err) {
        console.error('LoginPage: failed to refresh session after login', err)
        setError('Login succeeded, but session refresh failed. Please try again.')
        return
      }
      if (!user) {
        console.error('LoginPage: refreshSession returned no user after login')
        setError('Login succeeded, but session could not be loaded. Please try again.')
        return
      }
      navigate(redirectTo, { replace: true })
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
          {oidcError && (
            <div
              role="alert"
              aria-live="assertive"
              style={{
                display: 'flex',
                alignItems: 'flex-start',
                gap: 6,
                color: 'var(--danger)',
                fontSize: '12px',
                marginBottom: 12,
              }}
            >
              <AlertCircle size={14} style={{ marginTop: 1, flexShrink: 0 }} />
              <span>{oidcError} Use password login or contact your administrator.</span>
            </div>
          )}

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
              marginBottom: oidcProviders.length > 0 ? 8 : 0,
            }}
          >
            {loading ? 'LOGGING IN...' : 'LOGIN'}
          </button>

          {oidcProviders.length > 0 && (
            <div style={{ display: 'grid', gap: 8 }}>
              {oidcProviders.map(provider => (
                <a
                  key={provider.id}
                  href={`/api/auth/oidc/${encodeURIComponent(provider.id)}/login`}
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
                  {`SIGN IN WITH ${provider.name.toUpperCase()}`}
                </a>
              ))}
            </div>
          )}
        </form>
      </div>
    </div>
  )
}
