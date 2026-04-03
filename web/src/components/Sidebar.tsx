import { NavLink, useNavigate } from 'react-router-dom'
import { useNodePulse } from './NodePulse'

const NAV_ITEMS = [
  { label: 'TIMELINE', to: '/timeline' },
  { label: 'NODES', to: '/nodes', pulse: true },
] as const

const ADMIN_ITEMS = [
  { label: 'WEBHOOKS', to: '/webhooks' },
  { label: 'ACCOUNT', to: '/account' },
  { label: 'ADMIN', to: '/admin' },
  { label: 'DIAGNOSTICS', to: '/diagnostics' },
] as const

const activeStyle: React.CSSProperties = {
  borderLeft: '2px solid var(--accent)',
  paddingLeft: '14px',
  color: 'var(--accent)',
}

const baseStyle: React.CSSProperties = {
  display: 'block',
  padding: '6px 16px',
  fontSize: '11px',
  letterSpacing: '0.1em',
  color: 'var(--muted)',
  textDecoration: 'none',
  lineHeight: '1.4',
}

export default function Sidebar() {
  const { onlineCount, totalCount } = useNodePulse()
  const navigate = useNavigate()

  const username = (() => {
    try {
      const token = localStorage.getItem('token')
      if (!token) return ''
      const payload = JSON.parse(atob(token.split('.')[1]))
      return payload.username ?? ''
    } catch {
      return ''
    }
  })()

  function nodeBadge() {
    if (totalCount === 0) return <span style={{ color: 'var(--muted)', marginLeft: 4 }}>[—]</span>
    const color = onlineCount < totalCount ? '#FF4444' : 'var(--accent)'
    return <span style={{ color, marginLeft: 4 }}>[{onlineCount}/{totalCount}]</span>
  }

  return (
    <div
      style={{
        width: 180,
        minHeight: '100vh',
        background: 'var(--surface)',
        borderRight: '1px solid var(--border)',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      <div
        style={{
          padding: '16px',
          fontSize: '12px',
          letterSpacing: '0.15em',
          color: 'var(--accent)',
          fontWeight: 'bold',
        }}
      >
        BLACKBOX
      </div>

      <div style={{ borderTop: '1px solid var(--border)' }} />

      <nav style={{ paddingTop: 8 }}>
        {NAV_ITEMS.map(item => (
          <NavLink
            key={item.to}
            to={item.to}
            style={({ isActive }) => ({ ...baseStyle, ...(isActive ? activeStyle : {}) })}
          >
            {item.label}
            {'pulse' in item && item.pulse && nodeBadge()}
          </NavLink>
        ))}
      </nav>

      <div style={{ borderTop: '1px solid var(--border)', margin: '8px 0' }} />

      <nav>
        {ADMIN_ITEMS.map(item => (
          <NavLink
            key={item.to}
            to={item.to}
            style={({ isActive }) => ({ ...baseStyle, ...(isActive ? activeStyle : {}) })}
          >
            {item.label}
          </NavLink>
        ))}
      </nav>

      <div style={{ flex: 1 }} />

      <div style={{ borderTop: '1px solid var(--border)', padding: '10px 16px' }}>
        <span
          onClick={() => {
            localStorage.removeItem('token')
            navigate('/login')
          }}
          style={{ ...baseStyle, padding: 0, cursor: 'pointer', display: 'block' }}
          title="Logout"
        >
          {username || 'USER'}
        </span>
      </div>
    </div>
  )
}
