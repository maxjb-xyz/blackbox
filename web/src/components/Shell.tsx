import { useEffect } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import Topbar from './Topbar'
import BottomNav from './BottomNav'
import { NodePulseProvider } from './NodePulse'
import { WebSocketProvider } from './WebSocketProvider'
import { useSession } from '../session'

export default function Shell() {
  const navigate = useNavigate()
  const location = useLocation()
  const { user, loading } = useSession()

  useEffect(() => {
    if (!loading && !user) {
      const redirectTo = encodeURIComponent(location.pathname + location.search)
      navigate(`/login?redirect_to=${redirectTo}`, { replace: true })
    }
  }, [loading, user, navigate, location.pathname, location.search])

  if (loading || !user) return null

  return (
    <WebSocketProvider>
      <NodePulseProvider>
        <div style={{ display: 'flex', flexDirection: 'column', minHeight: '100vh' }}>
          <Topbar />
          <main className="shell-main" style={{ flex: 1, overflow: 'auto', background: 'var(--bg)' }}>
            <div
              className="terminal-startup"
              style={{ width: '100%', minHeight: '100%' }}
            >
              <Outlet />
            </div>
          </main>
          <BottomNav />
        </div>
      </NodePulseProvider>
    </WebSocketProvider>
  )
}
