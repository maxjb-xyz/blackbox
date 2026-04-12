import { useCallback, useEffect, useRef, useState } from 'react'
import { NavLink } from 'react-router-dom'
import { Activity, AlertTriangle, Server } from 'lucide-react'
import { fetchIncidentSummary } from '../api/client'
import { useWebSocketContext } from './WebSocketProvider'

const MOBILE_MQ = '(max-width: 640px)'

export default function BottomNav() {
  const { status, lastMessage } = useWebSocketContext()
  const [openCount, setOpenCount] = useState(0)
  const [hasConfirmed, setHasConfirmed] = useState(false)
  const [isMobile, setIsMobile] = useState(
    () => typeof window !== 'undefined' && window.matchMedia(MOBILE_MQ).matches
  )
  const reqIdRef = useRef(0)

  useEffect(() => {
    const mq = window.matchMedia(MOBILE_MQ)
    const handler = (e: MediaQueryListEvent) => setIsMobile(e.matches)
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [])

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
    if (!isMobile) return
    refreshCount()
  }, [isMobile, refreshCount])

  useEffect(() => {
    if (!isMobile || !lastMessage) return
    const { type } = lastMessage
    if (type === 'incident_opened' || type === 'incident_updated' || type === 'incident_resolved') {
      refreshCount()
    }
  }, [isMobile, lastMessage, refreshCount])

  const previousStatusRef = useRef(status)
  useEffect(() => {
    const previousStatus = previousStatusRef.current
    if (isMobile && status === 'connected' && previousStatus !== 'connected') {
      refreshCount()
    }
    previousStatusRef.current = status
  }, [isMobile, refreshCount, status])

  const badgeColor = hasConfirmed ? 'var(--danger)' : 'var(--warning)'

  if (!isMobile) return null

  return (
    <nav className="bottom-nav" aria-label="Mobile navigation">
      <NavLink
        to="/incidents"
        className={({ isActive }) => `bottom-nav-item${isActive ? ' active' : ''}`}
      >
        <span style={{ position: 'relative', display: 'inline-flex' }}>
          <AlertTriangle size={18} />
          {openCount > 0 && (
            <span
              style={{
                position: 'absolute',
                top: -4,
                right: -6,
                fontSize: 9,
                lineHeight: 1,
                padding: '1px 3px',
                border: `1px solid ${badgeColor}`,
                color: badgeColor,
                background: 'var(--bg)',
              }}
            >
              {openCount > 99 ? '99+' : openCount}
            </span>
          )}
        </span>
        INCIDENTS
      </NavLink>

      <NavLink
        to="/timeline"
        className={({ isActive }) => `bottom-nav-item${isActive ? ' active' : ''}`}
      >
        <Activity size={18} />
        TIMELINE
      </NavLink>

      <NavLink
        to="/nodes"
        className={({ isActive }) => `bottom-nav-item${isActive ? ' active' : ''}`}
      >
        <Server size={18} />
        NODES
      </NavLink>
    </nav>
  )
}
