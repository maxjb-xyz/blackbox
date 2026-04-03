import { useEffect } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import Sidebar from './Sidebar'
import { NodePulseProvider } from './NodePulse'

export default function Shell() {
  const navigate = useNavigate()
  const location = useLocation()

  useEffect(() => {
    const token = localStorage.getItem('token')
    if (!token) {
      const redirectTo = encodeURIComponent(location.pathname + location.search)
      navigate(`/login?redirect_to=${redirectTo}`, { replace: true })
    }
  }, [navigate, location.pathname, location.search])

  const token = localStorage.getItem('token')
  if (!token) return null

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
