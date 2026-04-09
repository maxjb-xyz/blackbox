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
  const { user, loading: sessionLoading, refreshSession } = useSession()
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

  useEffect(() => {
    if (!sessionLoading && user) {
      navigate(redirectTo, { replace: true })
    }
  }, [navigate, redirectTo, sessionLoading, user])

  if (sessionLoading || user) {
    return null
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setLoading(true)
    try {
      await login(username, password)
      // Deliberately shadow the outer user with the local user from refreshSession so this handler can
      // inspect the immediate return value; refreshSession can return null for a superseded request even
      // when the outer user updates later, so the setError path here is intentional.
      const user = await refreshSession()
      if (!user) {
        console.error('LoginPage: refreshSession returned no user after login')
        setError('Login succeeded, but session could not be loaded. Please try again.')
        return
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="auth-page">
      <div className="auth-card">
        <div className="auth-card-header">
          <Terminal size={14} style={{ color: 'var(--accent)', flexShrink: 0 }} />
          <span className="auth-title">BLACKBOX</span>
          <span className="auth-sep">/</span>
          <span className="auth-title-page">LOGIN</span>
          <span className="cursor-blink" style={{ fontSize: '12px', marginLeft: 'auto' }}>_</span>
        </div>

        <div className="auth-card-body">
          <form onSubmit={handleSubmit}>
            {oidcError && (
              <div role="alert" aria-live="assertive" className="auth-error">
                <AlertCircle size={13} className="auth-error-icon" />
                <span>{oidcError} Use password login or contact your administrator.</span>
              </div>
            )}

            <div className="auth-field">
              <label htmlFor="username" className="auth-label">USERNAME</label>
              <input
                id="username"
                className="auth-input"
                type="text"
                value={username}
                onChange={e => setUsername(e.target.value)}
                required
                autoFocus
              />
            </div>

            <div className="auth-field" style={{ marginBottom: 18 }}>
              <label htmlFor="password" className="auth-label">PASSWORD</label>
              <input
                id="password"
                className="auth-input"
                type="password"
                value={password}
                onChange={e => setPassword(e.target.value)}
                required
              />
            </div>

            {error && (
              <div role="alert" aria-live="assertive" className="auth-error">
                <AlertCircle size={13} className="auth-error-icon" />
                <span>{error}</span>
              </div>
            )}

            <button
              type="submit"
              disabled={loading}
              className="auth-btn-primary"
              style={{ marginBottom: oidcProviders.length > 0 ? 8 : 0 }}
            >
              {loading ? 'LOGGING IN...' : 'LOGIN'}
            </button>

            {oidcProviders.length > 0 && (
              <div style={{ display: 'grid', gap: 6, marginTop: 8 }}>
                {oidcProviders.map(provider => (
                  <a
                    key={provider.id}
                    href={`/api/auth/oidc/${encodeURIComponent(provider.id)}/login`}
                    className="auth-btn-oidc"
                  >
                    {`SIGN IN WITH ${provider.name.toUpperCase()}`}
                  </a>
                ))}
              </div>
            )}
          </form>
        </div>
      </div>

      <div className="auth-tagline">FLIGHT RECORDER · OPERATIONAL</div>
    </div>
  )
}
