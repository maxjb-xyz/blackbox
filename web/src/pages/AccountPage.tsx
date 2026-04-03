import { useNavigate } from 'react-router-dom'

export default function AccountPage() {
  const navigate = useNavigate()

  const username = (() => {
    try {
      const token = localStorage.getItem('token')
      if (!token) return '—'
      const payload = JSON.parse(atob(token.split('.')[1]))
      return payload.username ?? '—'
    } catch {
      return '—'
    }
  })()

  function handleLogout() {
    localStorage.removeItem('token')
    navigate('/login', { replace: true })
  }

  return (
    <div>
      <div style={{ padding: '12px 16px', borderBottom: '1px solid var(--border)', background: 'var(--surface)' }}>
        <span style={{ color: 'var(--muted)', fontSize: '11px', letterSpacing: '0.1em' }}>ACCOUNT</span>
      </div>
      <div style={{ padding: 16 }}>
        <div style={{ marginBottom: 16, fontSize: '12px', color: 'var(--muted)' }}>
          LOGGED IN AS <span style={{ color: 'var(--text)' }}>{username}</span>
        </div>
        <button
          onClick={handleLogout}
          style={{
            background: 'transparent',
            border: '1px solid var(--border)',
            color: '#FF4444',
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
