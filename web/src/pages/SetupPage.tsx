import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Terminal, AlertCircle } from 'lucide-react'
import { bootstrap } from '../api/client'

export default function SetupPage() {
  const navigate = useNavigate()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setLoading(true)
    try {
      const token = await bootstrap(username, password)
      localStorage.setItem('token', token)
      navigate('/')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center" style={{ background: 'var(--bg)' }}>
      <div className="w-full max-w-sm" style={{ border: '1px solid var(--border)', padding: '2rem' }}>
        <div className="flex items-center gap-2 mb-6">
          <Terminal size={16} style={{ color: 'var(--accent)' }} />
          <span style={{ color: 'var(--accent)', fontSize: '11px', letterSpacing: '0.1em' }}>
            BLACKBOX / INITIAL SETUP
          </span>
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

          {error && (
            <div className="flex items-center gap-2" style={{ color: '#FF4444', fontSize: '12px', marginBottom: '1rem' }}>
              <AlertCircle size={12} />
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
            }}
          >
            {loading ? 'CREATING...' : 'CREATE ADMIN'}
          </button>
        </form>
      </div>
    </div>
  )
}
