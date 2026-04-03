import { NavLink, useNavigate } from 'react-router-dom'
import { useNodePulse } from './NodePulse'
import { getTokenIsAdmin, getTokenUsername } from '../utils/auth'

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
  const navigate = useNavigate()
  const username = getTokenUsername('')
  const isAdmin = getTokenIsAdmin()

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
    const colorClassName = onlineCount < totalCount ? 'text-[var(--danger)]' : 'text-[var(--accent)]'
    return <span className={`ml-1 ${colorClassName}`}>[{onlineCount}/{totalCount}]</span>
  }

  return (
    <div className="flex min-h-screen w-[180px] flex-col border-r border-[var(--border)] bg-[#0B0B0B] font-mono">
      <div className="px-4 py-4 text-xs font-bold tracking-[0.15em] text-[var(--accent)]">
        BLACKBOX
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
            localStorage.removeItem('token')
            navigate('/login')
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
