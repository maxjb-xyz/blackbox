import { useEffect, useState } from 'react'
import { AlertCircle, Terminal } from 'lucide-react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { fetchOIDCProviders, register } from '../api/client'
import { useSession } from '../session'

interface OIDCProvider { id: string; name: string }

export default function RegisterPage() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { user, loading: sessionLoading, refreshSession } = useSession()
  const inviteCodeFromUrl = searchParams.get('code')?.trim() ?? ''
  const inviteCodeReadonly = inviteCodeFromUrl.length > 0

  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [inviteCode, setInviteCode] = useState(inviteCodeFromUrl)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [oidcProviders, setOIDCProviders] = useState<OIDCProvider[]>([])

  useEffect(() => {
    setInviteCode(inviteCodeFromUrl)
  }, [inviteCodeFromUrl])

  useEffect(() => {
    if (!sessionLoading && user) {
      navigate('/', { replace: true })
    }
  }, [navigate, sessionLoading, user])

  useEffect(() => {
    fetchOIDCProviders()
      .then(data => setOIDCProviders(data.providers))
      .catch(() => { /* OIDC providers are optional */ })
  }, [])

  if (sessionLoading || user) {
    return null
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setLoading(true)
    try {
      await register(username, email, password, inviteCode)
      let user = null
      user = await refreshSession()
      if (!user) {
        setError('Registration succeeded, but session refresh failed. Please sign in.')
        return
      }
      navigate('/timeline', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Registration failed')
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
          <span className="auth-title-page">REGISTER</span>
          <span className="cursor-blink" style={{ fontSize: '12px', marginLeft: 'auto' }}>_</span>
        </div>

        <div className="auth-card-body">
          <form onSubmit={handleSubmit}>
            <div className="auth-field">
              <label htmlFor="register-username" className="auth-label">USERNAME</label>
              <input
                id="register-username"
                className="auth-input"
                type="text"
                value={username}
                onChange={e => setUsername(e.target.value)}
                required
                autoFocus
              />
            </div>

            <div className="auth-field">
              <label htmlFor="register-email" className="auth-label">EMAIL</label>
              <input
                id="register-email"
                className="auth-input"
                type="email"
                value={email}
                onChange={e => setEmail(e.target.value)}
                required
              />
            </div>

            <div className="auth-field">
              <label htmlFor="register-password" className="auth-label">PASSWORD</label>
              <input
                id="register-password"
                className="auth-input"
                type="password"
                value={password}
                onChange={e => setPassword(e.target.value)}
                required
              />
            </div>

            <div className="auth-field" style={{ marginBottom: 18 }}>
              <label htmlFor="register-invite-code" className="auth-label">INVITE CODE</label>
              <input
                id="register-invite-code"
                className="auth-input"
                type="text"
                value={inviteCode}
                onChange={e => setInviteCode(e.target.value)}
                required
                readOnly={inviteCodeReadonly}
              />
            </div>

            {error && (
              <div
                role="alert"
                aria-live="assertive"
                aria-atomic="true"
                className="auth-error"
              >
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
              {loading ? 'CREATING ACCOUNT...' : 'CREATE ACCOUNT'}
            </button>

            {oidcProviders.length > 0 && (
              <div style={{ display: 'grid', gap: 6, marginTop: 8, marginBottom: 8 }}>
                {oidcProviders.map(provider => {
                  const href = inviteCode
                    ? `/api/auth/oidc/${encodeURIComponent(provider.id)}/login?invite_code=${encodeURIComponent(inviteCode)}`
                    : `/api/auth/oidc/${encodeURIComponent(provider.id)}/login`
                  return (
                    <a key={provider.id} href={href} className="auth-btn-oidc">
                      {`SIGN IN WITH ${provider.name.toUpperCase()}`}
                    </a>
                  )
                })}
              </div>
            )}

            <hr className="auth-divider" />

            <Link to="/login" className="auth-footer-link">
              Already have an account? Sign in →
            </Link>
          </form>
        </div>
      </div>

      <div className="auth-tagline">FLIGHT RECORDER · OPERATIONAL</div>
    </div>
  )
}
