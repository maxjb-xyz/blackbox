import { useNavigate } from 'react-router-dom'
import { useSession } from '../session'

export default function AccountPage() {
  const navigate = useNavigate()
  const { user, logout } = useSession()
  const username = user?.username ?? ''

  function handleLogout() {
    void logout().finally(() => {
      navigate('/login', { replace: true })
    })
  }

  return (
    <div>
      <div style={{ padding: '18px 24px', borderBottom: '1px solid var(--border)', background: 'var(--surface)' }}>
        <span style={{ color: 'var(--muted)', fontSize: '11px', letterSpacing: '0.1em' }}>ACCOUNT</span>
      </div>
      <div style={{ padding: '24px', maxWidth: 960, margin: '0 auto' }}>
        <div style={{ marginBottom: 16, fontSize: '12px', color: 'var(--muted)' }}>
          LOGGED IN AS <span style={{ color: 'var(--text)' }}>{username || '—'}</span>
        </div>
        <button
          onClick={handleLogout}
          style={{
            background: 'transparent',
            border: '1px solid var(--border)',
            color: 'var(--danger)',
            padding: '8px 16px',
            fontFamily: 'inherit',
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
