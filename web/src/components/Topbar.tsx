import { useCallback, useEffect, useRef, useState } from 'react'
import { NavLink } from 'react-router-dom'
import { Activity, AlertTriangle, ChevronDown, Server } from 'lucide-react'
import { fetchIncidentSummary } from '../api/client'
import { useNodePulse } from './NodePulse'
import { useWebSocketContext } from './WebSocketProvider'
import { useSession } from '../session'
import UserDropdown from './UserDropdown'

export default function Topbar() {
  const { onlineCount, totalCount } = useNodePulse()
  const { status, reconnect, lastConnectedAt, lastMessage } = useWebSocketContext()
  const { user } = useSession()
  const [dropdownOpen, setDropdownOpen] = useState(false)
  const [openCount, setOpenCount] = useState(0)
  const [hasConfirmed, setHasConfirmed] = useState(false)
  const reqIdRef = useRef(0)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const previousStatusRef = useRef(status)
  const dropdownId = 'topbar-user-menu'

  const refreshCount = useCallback(() => {
    const id = ++reqIdRef.current
    fetchIncidentSummary()
      .then(summary => {
        if (reqIdRef.current !== id) return
        setOpenCount(summary.openCount)
        setHasConfirmed(summary.hasConfirmedOpen)
      })
      .catch(() => {})
  }, [])

  useEffect(() => {
    refreshCount()
  }, [refreshCount])

  useEffect(() => {
    if (!lastMessage) return
    const { type } = lastMessage
    if (type === 'incident_opened' || type === 'incident_updated' || type === 'incident_resolved') {
      refreshCount()
    }
  }, [lastMessage, refreshCount])

  useEffect(() => {
    const previousStatus = previousStatusRef.current
    if (status === 'connected' && previousStatus !== 'connected') {
      refreshCount()
    }
    previousStatusRef.current = status
  }, [refreshCount, status])

  const wsConnected = status === 'connected'
  const wsConnecting = status === 'connecting'
  const nodeColor = totalCount > 0 && onlineCount < totalCount ? 'var(--danger)' : 'var(--success)'

  const wsTitle = wsConnected
    ? `CONNECTED${lastConnectedAt ? ` \u2014 LAST MSG ${lastConnectedAt.toLocaleTimeString()}` : ''}`
    : wsConnecting ? 'CONNECTING...' : 'DISCONNECTED \u2014 CLICK TO RECONNECT'

  function navClass(isActive: boolean) {
    return [
      'flex items-center gap-2 h-full px-[18px] text-[12px] tracking-[0.12em] no-underline border-b-2 transition-colors',
      isActive
        ? 'text-[#E8E8E8] border-b-[var(--accent)]'
        : 'text-[#666] border-b-transparent hover:text-[#aaa]',
    ].join(' ')
  }

  return (
    <header
      className="sticky top-0 z-50 flex h-[52px] flex-shrink-0 items-center border-b border-[#242424] bg-[#0D0D0D] px-6"
    >
      {/* Logo */}
      <span
        className="mr-9 text-[14px] font-bold tracking-[0.18em]"
        style={{ color: 'var(--accent)' }}
      >
        BLACKBOX<span className="cursor-blink" aria-hidden="true">|</span>
      </span>

      {/* Primary nav */}
      <nav className="flex h-full flex-1 items-stretch gap-0.5">
        <NavLink to="/incidents" className={({ isActive }) => navClass(isActive)}>
          <AlertTriangle size={14} />
          INCIDENTS
          {openCount > 0 && (
            <span
              className="border px-[5px] py-[1px] text-[10px] leading-[1.4] tracking-[0.08em]"
              style={{
                color: hasConfirmed ? 'var(--danger)' : 'var(--warning)',
                borderColor: hasConfirmed ? 'var(--danger)' : 'var(--warning)',
              }}
            >
              {openCount}
            </span>
          )}
        </NavLink>

        <NavLink to="/timeline" className={({ isActive }) => navClass(isActive)}>
          <Activity size={14} />
          TIMELINE
        </NavLink>

        <NavLink to="/nodes" className={({ isActive }) => navClass(isActive)}>
          <Server size={14} />
          NODES
          {totalCount > 0 && (
            <span className="text-[11px] tracking-[0.08em]" style={{ color: nodeColor }}>
              [{onlineCount}/{totalCount}]
            </span>
          )}
        </NavLink>
      </nav>

      {/* Right side */}
      <div className="flex items-center gap-[18px]">
        {/* WS status */}
        <button
          type="button"
          title={wsTitle}
          aria-label={wsTitle}
          onClick={status === 'disconnected' ? reconnect : undefined}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 7,
            border: 'none',
            background: 'transparent',
            fontSize: 11,
            letterSpacing: '0.1em',
            cursor: status === 'disconnected' ? 'pointer' : 'default',
            color: wsConnected ? 'var(--success)' : wsConnecting ? 'var(--warning)' : 'var(--danger)',
            padding: 0,
            fontFamily: 'inherit',
          }}
        >
          <span
            className={wsConnected ? 'pulse-dot' : undefined}
            style={{
              display: 'inline-block',
              width: 8,
              height: 8,
              background: wsConnected ? 'var(--success)' : wsConnecting ? 'var(--warning)' : 'var(--danger)',
              flexShrink: 0,
            }}
          />
          {wsConnected ? 'LIVE' : wsConnecting ? 'CONNECTING' : 'OFFLINE'}
        </button>

        {/* User dropdown */}
        <div style={{ position: 'relative' }}>
          <button
            ref={triggerRef}
            type="button"
            aria-haspopup="menu"
            aria-expanded={dropdownOpen}
            aria-controls={dropdownId}
            onClick={() => setDropdownOpen(v => !v)}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              border: '1px solid transparent',
              background: 'transparent',
              color: '#888',
              fontSize: 12,
              letterSpacing: '0.1em',
              cursor: 'pointer',
              padding: '4px 8px',
              fontFamily: 'inherit',
              transition: 'color 0.15s, border-color 0.15s',
            }}
            onMouseEnter={e => { (e.currentTarget as HTMLButtonElement).style.color = '#ccc'; (e.currentTarget as HTMLButtonElement).style.borderColor = '#2a2a2a' }}
            onMouseLeave={e => { (e.currentTarget as HTMLButtonElement).style.color = '#888'; (e.currentTarget as HTMLButtonElement).style.borderColor = 'transparent' }}
          >
            {user?.username ?? 'USER'}
            <ChevronDown size={14} className="text-[#555]" />
          </button>
          {dropdownOpen && <UserDropdown id={dropdownId} onClose={() => setDropdownOpen(false)} triggerRef={triggerRef} />}
        </div>
      </div>
    </header>
  )
}
