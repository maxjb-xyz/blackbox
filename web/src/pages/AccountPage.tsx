import { useNavigate } from 'react-router-dom'
import { useSession } from '../session'

export default function AccountPage() {
  const navigate = useNavigate()
  const { user, logout } = useSession()
  const username = user?.username ?? ''
  const email = user?.email?.trim() ? user.email : '—'

  function handleLogout() {
    void logout().finally(() => {
      navigate('/login', { replace: true })
    })
  }

  return (
    <div style={{ minHeight: '100%', background: '#0B0B0B', fontFamily: 'JetBrains Mono, Fira Code, Cascadia Code, ui-monospace, monospace' }}>
      <div style={{ padding: '8px 10px', borderBottom: '1px solid var(--border)', background: '#0B0B0B' }}>
        <span style={{ color: 'var(--muted)', fontSize: '11px', letterSpacing: '0.1em' }}>ACCOUNT</span>
      </div>
      <div style={{ padding: '10px 16px', maxWidth: 960, margin: '0 auto', background: '#0B0B0B', fontFamily: 'JetBrains Mono, Fira Code, Cascadia Code, ui-monospace, monospace' }}>
        <div style={{ marginBottom: 16, fontSize: '12px', color: 'var(--muted)' }}>
          LOGGED IN AS <span style={{ color: 'var(--text)' }}>{username || '—'}</span>
        </div>
        <div style={{ marginBottom: 16, fontSize: '12px', color: 'var(--muted)' }}>
          EMAIL <span style={{ color: 'var(--text)' }}>{email}</span>
        </div>
        <button
          onClick={handleLogout}
          style={{
            background: 'transparent',
            border: '1px solid var(--border)',
            color: 'var(--danger)',
            padding: '8px 16px',
            fontFamily: 'JetBrains Mono, Fira Code, Cascadia Code, ui-monospace, monospace',
            fontSize: '12px',
            letterSpacing: '0.1em',
            cursor: 'pointer',
          }}
        >
          LOGOUT
        </button>
      </div>
    </div>
  )
}
