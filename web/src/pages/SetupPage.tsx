import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Terminal, AlertCircle, CheckCircle, XCircle, Loader } from 'lucide-react'
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
    <div className="min-h-screen flex items-center justify-center" style={{ background: 'var(--bg)' }}>
      <div className="w-full max-w-sm" style={{ border: '1px solid var(--border)', padding: '2rem' }}>
        <div className="flex items-center gap-2 mb-6">
          <Terminal size={16} style={{ color: 'var(--muted)' }} />
          <span style={{ color: 'var(--muted)', fontSize: '12px', letterSpacing: '0.1em' }}>
            BLACKBOX / INITIAL SETUP
          </span>
        </div>

        <div style={{ marginBottom: '1.5rem', border: '1px solid var(--border)', padding: '0.75rem' }}>
          <div style={{ color: 'var(--muted)', fontSize: '10px', letterSpacing: '0.1em', marginBottom: '0.5rem' }}>
            SYSTEM HEALTH
          </div>
          {healthLoading ? (
            <div className="flex items-center gap-2" style={{ color: 'var(--muted)', fontSize: '12px' }}>
              <Loader size={12} />
              <span>Checking…</span>
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
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

        <p style={{ color: 'var(--muted)', fontSize: '12px', marginBottom: '1.5rem', lineHeight: '1.6' }}>
          No admin account found. Create the first admin user to continue.
        </p>

        <form onSubmit={handleSubmit}>
          <div style={{ marginBottom: '1rem' }}>
            <label style={{ display: 'block', color: 'var(--muted)', fontSize: '11px', marginBottom: '4px', letterSpacing: '0.05em' }}>
              USERNAME
            </label>
            <input
              type="text"
              value={username}
              onChange={e => setUsername(e.target.value)}
              required
              autoFocus
              style={{
                width: '100%',
                background: 'var(--surface)',
                border: '1px solid var(--border)',
                color: 'var(--text)',
                padding: '8px 10px',
                fontFamily: 'inherit',
                fontSize: '13px',
                outline: 'none',
              }}
            />
          </div>

          <div style={{ marginBottom: '1rem' }}>
            <label style={{ display: 'block', color: 'var(--muted)', fontSize: '11px', marginBottom: '4px', letterSpacing: '0.05em' }}>
              EMAIL
            </label>
            <input
              type="email"
              value={email}
              onChange={e => setEmail(e.target.value)}
              required
              style={{
                width: '100%',
                background: 'var(--surface)',
                border: '1px solid var(--border)',
                color: 'var(--text)',
                padding: '8px 10px',
                fontFamily: 'inherit',
                fontSize: '13px',
                outline: 'none',
              }}
            />
          </div>

          <div style={{ marginBottom: '1.5rem' }}>
            <label style={{ display: 'block', color: 'var(--muted)', fontSize: '11px', marginBottom: '4px', letterSpacing: '0.05em' }}>
              PASSWORD
            </label>
            <input
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              required
              style={{
                width: '100%',
                background: 'var(--surface)',
                border: '1px solid var(--border)',
                color: 'var(--text)',
                padding: '8px 10px',
                fontFamily: 'inherit',
                fontSize: '13px',
                outline: 'none',
              }}
            />
          </div>

          {!dbOK && !healthLoading && (
            <div className="flex items-center gap-2" style={{ color: '#FF4444', fontSize: '12px', marginBottom: '1rem' }}>
              <AlertCircle size={12} />
              <span>Database unavailable - cannot create account</span>
            </div>
          )}

          {error && (
            <div className="flex items-center gap-2" style={{ color: '#FF4444', fontSize: '12px', marginBottom: '1rem' }}>
              <AlertCircle size={12} />
              <span>{error}</span>
            </div>
          )}

          <button
            type="submit"
            disabled={!canSubmit}
            style={{
              width: '100%',
              background: canSubmit ? 'var(--accent)' : 'var(--border)',
              color: '#000',
              border: 'none',
              padding: '10px',
              fontFamily: 'inherit',
              fontSize: '12px',
              fontWeight: 'bold',
              letterSpacing: '0.1em',
              cursor: canSubmit ? 'pointer' : 'not-allowed',
            }}
          >
            {loading ? 'CREATING...' : 'CREATE ADMIN'}
          </button>
        </form>
      </div>
    </div>
  )
}

function HealthRow({ label, status, message }: { label: string; status: 'ok' | 'error' | 'warn'; message?: string }) {
  const color = status === 'ok' ? '#00FF41' : status === 'warn' ? '#FFA500' : '#FF4444'
  const Icon = status === 'ok' ? CheckCircle : XCircle

  return (
    <div className="flex items-center gap-2" style={{ fontSize: '11px' }}>
      <Icon size={12} style={{ color }} />
      <span style={{ color: 'var(--muted)' }}>{label}</span>
      <span style={{ color }}>
        {status.toUpperCase()}
        {message ? ` - ${message}` : ''}
      </span>
    </div>
  )
}
