import { useCallback, useEffect, useRef, useState } from 'react'
import {
  Activity,
  AlertTriangle,
  ExternalLink,
  LogOut,
  Server,
  Shield,
  User,
  Webhook,
  Wrench,
} from 'lucide-react'
import { NavLink, useNavigate } from 'react-router-dom'
import { fetchIncidents, type Incident } from '../api/client'
import { useNodePulse } from './NodePulse'
import { useWebSocketContext } from './WebSocketProvider'
import { useSession } from '../session'

const NAV_ITEMS = [
  { label: 'INCIDENTS', to: '/incidents', icon: AlertTriangle, badge: true },
  { label: 'TIMELINE', to: '/timeline', icon: Activity },
  { label: 'NODES', to: '/nodes', icon: Server, pulse: true },
] as const

const ADMIN_ITEMS = [
  { label: 'WEBHOOKS', to: '/webhooks', icon: Webhook },
  { label: 'ACCOUNT', to: '/account', icon: User },
  { label: 'ADMIN', to: '/admin', icon: Shield, adminOnly: true },
  { label: 'DIAGNOSTICS', to: '/diagnostics', icon: Wrench },
] as const

export default function Sidebar() {
  const { onlineCount, totalCount, loading, error, lastUpdated } = useNodePulse()
  const { status, lastConnectedAt, lastMessage, reconnect } = useWebSocketContext()
  const navigate = useNavigate()
  const { user, logout } = useSession()
  const [isLogoutHovered, setIsLogoutHovered] = useState(false)
  const [openCount, setOpenCount] = useState(0)
  const [hasMoreOpen, setHasMoreOpen] = useState(false)
  const [hasConfirmed, setHasConfirmed] = useState(false)
  const openIncidentSummaryReqIdRef = useRef(0)

  const username = user?.username ?? ''
  const isAdmin = user?.is_admin === true
  const connectionStatusTitle = status === 'connected'
    ? `CONNECTED${lastConnectedAt ? ` - LAST MSG ${lastConnectedAt.toLocaleTimeString()}` : ''}`
    : status === 'connecting'
      ? 'CONNECTING...'
      : 'DISCONNECTED - CLICK TO RECONNECT'
  const wsColorClassName = status === 'connected'
    ? 'text-[var(--success)]'
    : status === 'connecting'
      ? 'text-[#FF9900]'
      : 'text-[var(--danger)]'
  const wsLabel = status === 'connected' ? 'CONNECTED' : status === 'connecting' ? 'CONNECTING...' : 'DISCONNECTED'

  function navClassName(isActive: boolean) {
    return [
      'flex items-center justify-between gap-2 border-l-2 px-4 py-1 text-[13px] leading-[1.4] tracking-wider no-underline transition-colors',
      isActive ? 'border-l-[var(--accent)] pl-[14px] text-[var(--accent)]' : 'border-l-transparent text-[var(--muted)]',
    ].join(' ')
  }

  function nodeBadge() {
    if (loading && !lastUpdated) {
      return <span className="text-[11px] text-[var(--muted)]">[...]</span>
    }
    if (error) {
      return (
        <span className="text-[11px] text-[var(--danger)]" title={error.message}>
          [!]
        </span>
      )
    }
    if (totalCount === 0) return <span className="text-[11px] text-[var(--muted)]">[0]</span>
    const colorClassName = onlineCount < totalCount ? 'text-[var(--danger)]' : 'text-[var(--success)]'
    return <span className={`text-[11px] ${colorClassName}`}>[{onlineCount}/{totalCount}]</span>
  }

  const refreshOpenIncidentSummary = useCallback(() => {
    const requestId = openIncidentSummaryReqIdRef.current + 1
    openIncidentSummaryReqIdRef.current = requestId
    fetchIncidents({ status: 'open', limit: 200 })
      .then(page => {
        if (openIncidentSummaryReqIdRef.current !== requestId) return
        setOpenCount(page.incidents.length)
        setHasMoreOpen(page.has_more)
        setHasConfirmed(page.incidents.some(i => i.confidence === 'confirmed'))
      })
      .catch(() => {})
  }, [])

  useEffect(() => {
    refreshOpenIncidentSummary()
  }, [refreshOpenIncidentSummary])

  useEffect(() => {
    if (!lastMessage) return
    const { type, data } = lastMessage
    if (type === 'incident_opened' || type === 'incident_updated' || type === 'incident_resolved') {
      const inc = data as Incident
      if (inc.id) {
        refreshOpenIncidentSummary()
      }
    }
  }, [lastMessage, refreshOpenIncidentSummary])

  return (
    <div className="flex min-h-screen w-[200px] flex-col border-r border-[var(--border)] bg-[#0B0B0B] font-mono">
      <div className="flex items-center px-4 py-4">
        <span className="text-[12px] font-bold tracking-[0.15em] text-[var(--accent)]">
          BLACKBOX
          <span aria-hidden="true" className="cursor-blink">|</span>
        </span>
      </div>

      <div className="border-t border-[var(--border)]" />

      <nav className="pt-2">
        {NAV_ITEMS.map(item => {
          const Icon = item.icon
          return (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) => navClassName(isActive)}
            >
              <span className="flex min-w-0 items-center gap-2">
                <Icon size={14} />
                <span>{item.label}</span>
              </span>
              {'badge' in item && item.badge && openCount > 0 && (
                <span
                  style={{
                    fontSize: 11,
                    color: hasConfirmed ? 'var(--danger)' : 'var(--warning)',
                  }}
                >
                  [{hasMoreOpen ? '200+' : openCount}]
                </span>
              )}
              {'pulse' in item && item.pulse && nodeBadge()}
            </NavLink>
          )
        })}
      </nav>

      <div className="my-2 border-t border-[var(--border)]" />

      <nav>
        {ADMIN_ITEMS.filter(item => !('adminOnly' in item) || !item.adminOnly || isAdmin).map(item => {
          const Icon = item.icon
          return (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) => navClassName(isActive)}
            >
              <span className="flex min-w-0 items-center gap-2">
                <Icon size={14} />
                <span>{item.label}</span>
              </span>
            </NavLink>
          )
        })}
      </nav>

      <div className="flex-1" />

      <div className="border-t border-[var(--border)] py-[10px]">
        <button
          type="button"
          title={connectionStatusTitle}
          aria-label={connectionStatusTitle}
          aria-disabled={status !== 'disconnected'}
          onClick={status === 'disconnected' ? reconnect : undefined}
          className={[
            'flex w-full items-center gap-2 border-none bg-transparent px-4 py-1 text-left text-[13px] leading-[1.4] tracking-wider',
            wsColorClassName,
            status === 'disconnected' ? 'cursor-pointer' : 'cursor-default',
          ].join(' ')}
        >
          <span aria-hidden="true">●</span>
          <span>{wsLabel}</span>
        </button>

        <a
          href="https://github.com/maxjb-xyz/blackbox/issues/new"
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center gap-2 px-4 py-1 text-xs leading-[1.4] tracking-wider text-[var(--muted)] no-underline transition-colors hover:text-[var(--text)]"
        >
          <ExternalLink size={12} />
          <span>REPORT ISSUE</span>
        </a>

        <div className="flex items-center justify-between px-4 py-[10px]">
          <NavLink
            to="/account"
            className="text-xs leading-[1.4] tracking-wider text-[var(--muted)] no-underline transition-colors hover:text-[var(--text)]"
          >
            {username || 'USER'}
          </NavLink>

          <button
            type="button"
            title="Logout"
            aria-label="Logout"
            onMouseEnter={() => setIsLogoutHovered(true)}
            onMouseLeave={() => setIsLogoutHovered(false)}
            onClick={() => {
              void logout().finally(() => {
                navigate('/login')
              })
            }}
            className={[
              'flex h-5 w-5 items-center justify-center border-none bg-transparent p-0 transition-colors',
              isLogoutHovered ? 'text-[var(--danger)]' : 'text-[var(--muted)]',
            ].join(' ')}
          >
            <LogOut size={12} />
          </button>
        </div>
      </div>
    </div>
  )
}
