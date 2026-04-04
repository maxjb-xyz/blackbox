import { NavLink, useNavigate } from 'react-router-dom'
import { useNodePulse } from './NodePulse'
import { useWebSocketContext } from './WebSocketProvider'
import { useSession } from '../session'

const NAV_ITEMS = [
  { label: 'TIMELINE', to: '/timeline' },
  { label: 'NODES', to: '/nodes', pulse: true },
] as const

const ADMIN_ITEMS = [
  { label: 'WEBHOOKS', to: '/webhooks' },
  { label: 'ACCOUNT', to: '/account' },
  { label: 'ADMIN', to: '/admin', adminOnly: true },
  { label: 'DIAGNOSTICS', to: '/diagnostics' },
] as const

export default function Sidebar() {
  const { onlineCount, totalCount, loading, error, lastUpdated } = useNodePulse()
  const { status, lastConnectedAt, reconnect } = useWebSocketContext()
  const navigate = useNavigate()
  const { user, logout } = useSession()
  const username = user?.username ?? ''
  const isAdmin = user?.is_admin === true
  const connectionStatusLabel = status === 'connected'
    ? `connected${lastConnectedAt ? ` - last msg ${lastConnectedAt.toLocaleTimeString()}` : ''}`
    : status === 'connecting'
      ? 'connecting'
      : 'disconnected - reconnect available'

  function navClassName(isActive: boolean) {
    return [
      'block border-l-2 px-4 py-1 text-xs leading-[1.4] tracking-wider no-underline transition-colors',
      isActive ? 'border-l-[var(--accent)] pl-[14px] text-[var(--accent)]' : 'border-l-transparent text-[var(--muted)]',
    ].join(' ')
  }

  function nodeBadge() {
    if (loading && !lastUpdated) {
      return <span className="ml-1 text-[var(--muted)]">[...]</span>
    }
    if (error) {
      return (
        <span className="ml-1 text-[var(--danger)]" title={error.message}>
          [!]
        </span>
      )
    }
    if (totalCount === 0) return <span className="ml-1 text-[var(--muted)]">[0]</span>
    const colorClassName = onlineCount < totalCount ? 'text-[var(--danger)]' : 'text-[var(--success)]'
    return <span className={`ml-1 ${colorClassName}`}>[{onlineCount}/{totalCount}]</span>
  }

  return (
    <div className="flex min-h-screen w-[200px] flex-col border-r border-[var(--border)] bg-[#0B0B0B] font-mono">
      <div className="flex items-center gap-2 px-4 py-4">
        <span className="text-xs font-bold tracking-[0.15em] text-[var(--accent)]">BLACKBOX</span>
        <button
          type="button"
          title={connectionStatusLabel}
          aria-label={connectionStatusLabel}
          aria-busy={status === 'connecting'}
          aria-pressed={status === 'disconnected'}
          disabled={status !== 'disconnected'}
          onClick={status === 'disconnected' ? reconnect : undefined}
          style={{
            background: 'transparent',
            border: 'none',
            padding: 0,
            color: status === 'connected' ? '#00CC44' : status === 'connecting' ? '#FF9900' : '#FF3333',
            fontSize: 10,
            cursor: status === 'disconnected' ? 'pointer' : 'default',
            lineHeight: 1,
            display: 'inline-flex',
            alignItems: 'center',
            gap: 4,
          }}
        >
          <span aria-hidden="true">●</span>
          <span style={{
            position: 'absolute',
            width: 1,
            height: 1,
            padding: 0,
            margin: -1,
            overflow: 'hidden',
            clip: 'rect(0, 0, 0, 0)',
            whiteSpace: 'nowrap',
            border: 0,
          }}
          >
            {connectionStatusLabel}
          </span>
        </button>
      </div>

      <div className="border-t border-[var(--border)]" />

      <nav className="pt-2">
        {NAV_ITEMS.map(item => (
          <NavLink
            key={item.to}
            to={item.to}
            className={({ isActive }) => navClassName(isActive)}
          >
            {item.label}
            {'pulse' in item && item.pulse && nodeBadge()}
          </NavLink>
        ))}
      </nav>

      <div className="my-2 border-t border-[var(--border)]" />

      <nav>
        {ADMIN_ITEMS.filter(item => !('adminOnly' in item) || !item.adminOnly || isAdmin).map(item => (
          <NavLink
            key={item.to}
            to={item.to}
            className={({ isActive }) => navClassName(isActive)}
          >
            {item.label}
          </NavLink>
        ))}
      </nav>

      <div className="flex-1" />

      <div className="border-t border-[var(--border)] px-4 py-[10px]">
        <button
          type="button"
          onClick={() => {
            void logout().finally(() => {
              navigate('/login')
            })
          }}
          className="block w-full cursor-pointer border-none bg-transparent px-0 py-0 text-left text-xs leading-[1.4] tracking-wider text-[var(--muted)]"
          title="Logout"
        >
          {username || 'USER'}
        </button>
      </div>
    </div>
  )
}
