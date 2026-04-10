import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Terminal, AlertCircle, AlertTriangle, CheckCircle, XCircle, Loader } from 'lucide-react'
import { bootstrap, checkHealth, type HealthStatus } from '../api/client'
import { useSession } from '../session'

interface SetupPageProps {
  onBootstrapped?: () => void
}

export default function SetupPage({ onBootstrapped }: SetupPageProps) {
  const navigate = useNavigate()
  const { refreshSession } = useSession()
  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [health, setHealth] = useState<HealthStatus | null>(null)
  const [healthLoading, setHealthLoading] = useState(true)

  useEffect(() => {
    checkHealth()
      .then(setHealth)
      .catch(() => setHealth({ database: 'error', oidc: 'disabled', oidc_enabled: false }))
      .finally(() => setHealthLoading(false))
  }, [])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setLoading(true)
    try {
      await bootstrap(username, email, password)
      let user = null
      try {
        user = await refreshSession()
      } catch (err) {
        console.error('SetupPage: failed to refresh session after bootstrap', err)
        setError('Account created, but session refresh failed. Please log in.')
        return
      }
      if (!user) {
        console.error('SetupPage: refreshSession returned no user after bootstrap')
        setError('Account created, but session could not be loaded. Please log in.')
        return
      }
      onBootstrapped?.()
      navigate('/')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }

  const dbOK = health?.database === 'ok'
  const canSubmit = !loading && dbOK

  return (
    <div className="auth-page">
      <div className="auth-card">
        <div className="auth-card-header">
          <Terminal size={14} className="auth-header-icon" />
          <span className="auth-title">BLACKBOX</span>
          <span className="auth-sep">/</span>
          <span className="auth-title-page">INITIAL SETUP</span>
          <span className="cursor-blink auth-header-cursor">_</span>
        </div>

        <div className="auth-card-body">
          <div className="auth-health-box">
            <div className="auth-health-title">SYSTEM DIAGNOSTICS</div>
            {healthLoading ? (
              <div className="auth-health-row auth-health-loading">
                <Loader size={14} />
                <span>Checking…</span>
              </div>
            ) : (
              <div className="auth-health-stack">
                <HealthRow label="DATABASE" status={health?.database === 'ok' ? 'ok' : 'error'} />
                {health?.oidc_enabled && (
                  <HealthRow
                    label="OIDC"
                    status={health.oidc === 'ok' ? 'ok' : 'warn'}
                    message={health.oidc === 'unavailable' ? 'provider unreachable' : undefined}
                  />
                )}
              </div>
            )}
          </div>

          <p className="auth-setup-copy">
            No admin account found. Create the first admin user to continue.
          </p>

          <form onSubmit={handleSubmit}>
            <div className="auth-field">
              <label htmlFor="setup-username" className="auth-label">USERNAME</label>
              <input
                id="setup-username"
                type="text"
                className="auth-input"
                value={username}
                onChange={e => setUsername(e.target.value)}
                required
                autoFocus
              />
            </div>

            <div className="auth-field">
              <label htmlFor="setup-email" className="auth-label">EMAIL</label>
              <input
                id="setup-email"
                type="email"
                className="auth-input"
                value={email}
                onChange={e => setEmail(e.target.value)}
                required
              />
            </div>

            <div className="auth-field auth-field-last">
              <label htmlFor="setup-password" className="auth-label">PASSWORD</label>
              <input
                id="setup-password"
                type="password"
                className="auth-input"
                value={password}
                onChange={e => setPassword(e.target.value)}
                required
              />
            </div>

            {!dbOK && !healthLoading && (
              <div role="alert" aria-live="assertive" className="auth-error">
                <AlertCircle size={14} className="auth-error-icon" />
                <span>Database unavailable — cannot create account</span>
              </div>
            )}

            {error && (
              <div role="alert" aria-live="assertive" className="auth-error">
                <AlertCircle size={14} className="auth-error-icon" />
                <span>{error}</span>
              </div>
            )}

            <button
              type="submit"
              disabled={!canSubmit}
              className="auth-btn-primary"
            >
              {loading ? 'CREATING...' : 'CREATE ADMIN'}
            </button>
          </form>
        </div>
      </div>

      <div className="auth-tagline">FLIGHT RECORDER · OPERATIONAL</div>
    </div>
  )
}

function HealthRow({ label, status, message }: { label: string; status: 'ok' | 'error' | 'warn'; message?: string }) {
  const color = status === 'ok' ? 'var(--success)' : status === 'warn' ? 'var(--warning)' : 'var(--danger)'
  const Icon = status === 'ok' ? CheckCircle : status === 'warn' ? AlertTriangle : XCircle

  return (
    <div className="auth-health-row">
      <Icon size={14} style={{ color, flexShrink: 0 }} />
      <span className="auth-health-label">{label}</span>
      <span className="auth-health-status" style={{ color }}>
        {status.toUpperCase()}
        {message ? ` — ${message}` : ''}
      </span>
    </div>
  )
}
