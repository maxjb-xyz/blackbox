import { useEffect } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import Sidebar from './Sidebar'
import { NodePulseProvider } from './NodePulse'
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
    <NodePulseProvider>
      <div style={{ display: 'flex', minHeight: '100vh' }}>
        <Sidebar />
        <main style={{ flex: 1, overflow: 'auto', background: 'var(--bg)' }}>
          <Outlet />
        </main>
      </div>
    </NodePulseProvider>
  )
}
