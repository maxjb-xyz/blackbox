import { useCallback, useEffect, useRef, useState } from 'react'
import { NavLink } from 'react-router-dom'
import { Activity, AlertTriangle, Server } from 'lucide-react'
import { fetchIncidents } from '../api/client'
import type { Incident } from '../api/client'
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

  const refreshCount = useCallback(() => {
    const id = ++reqIdRef.current
    fetchIncidents({ status: 'open', limit: 200 })
      .then(page => {
        if (reqIdRef.current !== id) return
        setOpenCount(page.incidents.length)
        setHasConfirmed(page.incidents.some((i: Incident) => i.confidence === 'confirmed'))
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
          className={[
            'flex items-center gap-[7px] border-none bg-transparent text-[11px] tracking-[0.1em]',
            status === 'disconnected' ? 'cursor-pointer' : 'cursor-default',
            wsConnected
              ? 'text-[var(--success)]'
              : wsConnecting
                ? 'text-[var(--warning)]'
                : 'text-[var(--danger)]',
          ].join(' ')}
        >
          <span
            className={`ws-status-dot inline-block h-[7px] w-[7px]${wsConnected ? ' pulse-dot' : ''}`}
            style={{
              background: 'currentColor',
            }}
          />
          {wsConnected ? 'LIVE' : wsConnecting ? 'CONNECTING' : 'OFFLINE'}
        </button>

        {/* User dropdown */}
        <div style={{ position: 'relative' }}>
          <button
            type="button"
            onClick={() => setDropdownOpen(v => !v)}
            className="flex items-center gap-2 border border-transparent px-[10px] py-[6px] text-[12px] tracking-[0.1em] transition-all hover:border-[#2a2a2a] hover:text-[#bbb]"
            style={{ color: '#888', background: 'transparent' }}
          >
            {user?.username ?? 'USER'}
            <svg
              width="11"
              height="11"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              style={{ color: '#555' }}
            >
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </button>
          {dropdownOpen && <UserDropdown onClose={() => setDropdownOpen(false)} />}
        </div>
      </div>
    </header>
  )
}
