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
      try {
        user = await refreshSession()
      } catch (err) {
        console.error('RegisterPage: failed to refresh session after registration', err)
        setError('Registration succeeded, but session refresh failed. Please sign in.')
        return
      }
      if (!user) {
        console.error('RegisterPage: refreshSession returned no user after registration')
        setError('Registration succeeded, but session could not be loaded. Please sign in.')
        return
      }
      navigate('/timeline', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Registration failed')
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

  const labelStyle: React.CSSProperties = {
    display: 'block',
    color: 'var(--muted)',
    fontSize: '11px',
    marginBottom: 4,
    letterSpacing: '0.05em',
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
            BLACKBOX / REGISTER
          </span>
        </div>

        <form onSubmit={handleSubmit}>
          <div style={{ marginBottom: 12 }}>
            <label htmlFor="register-username" style={labelStyle}>USERNAME</label>
            <input
              id="register-username"
              className="login-input"
              type="text"
              value={username}
              onChange={e => setUsername(e.target.value)}
              required
              autoFocus
              style={inputStyle}
            />
          </div>

          <div style={{ marginBottom: 12 }}>
            <label htmlFor="register-email" style={labelStyle}>EMAIL</label>
            <input
              id="register-email"
              className="login-input"
              type="email"
              value={email}
              onChange={e => setEmail(e.target.value)}
              style={inputStyle}
            />
          </div>

          <div style={{ marginBottom: 12 }}>
            <label htmlFor="register-password" style={labelStyle}>PASSWORD</label>
            <input
              id="register-password"
              className="login-input"
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              required
              style={inputStyle}
            />
          </div>

          <div style={{ marginBottom: 20 }}>
            <label htmlFor="register-invite-code" style={labelStyle}>INVITE CODE</label>
            <input
              id="register-invite-code"
              className="login-input"
              type="text"
              value={inviteCode}
              onChange={e => setInviteCode(e.target.value)}
              required
              readOnly={inviteCodeReadonly}
              style={{
                ...inputStyle,
                color: inviteCodeReadonly ? 'var(--muted)' : 'var(--text)',
              }}
            />
          </div>

          {error && (
            <div
              role="alert"
              aria-live="assertive"
              aria-atomic="true"
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
              marginBottom: oidcProviders.length > 0 ? 8 : 12,
            }}
          >
            {loading ? 'CREATING ACCOUNT...' : 'CREATE ACCOUNT'}
          </button>

          {oidcProviders.length > 0 && (
            <div style={{ display: 'grid', gap: 8, marginBottom: 12 }}>
              {oidcProviders.map(provider => {
                const href = inviteCode
                  ? `/api/auth/oidc/${encodeURIComponent(provider.id)}/login?invite_code=${encodeURIComponent(inviteCode)}`
                  : `/api/auth/oidc/${encodeURIComponent(provider.id)}/login`
                return (
                  <a
                    key={provider.id}
                    href={href}
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
                )
              })}
            </div>
          )}

          <Link
            to="/login"
            style={{
              display: 'block',
              color: 'var(--muted)',
              fontSize: '12px',
              textAlign: 'center',
              textDecoration: 'none',
            }}
          >
            Already have an account? Sign in
          </Link>
        </form>
      </div>
    </div>
  )
}
