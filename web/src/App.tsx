import { useEffect, useState } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import SetupPage from './pages/SetupPage'
import { checkSetupStatus } from './api/client'

function AppRoutes() {
  const [bootstrapped, setBootstrapped] = useState<boolean | null>(null)

  useEffect(() => {
    checkSetupStatus()
      .then(s => setBootstrapped(s.bootstrapped))
      .catch(() => setBootstrapped(false))
  }, [])

  if (bootstrapped === null) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: '100vh', color: 'var(--muted)', fontSize: '12px' }}>
        loading...
      </div>
    )
  }

  return (
    <Routes>
      <Route path="/setup" element={<SetupPage onBootstrapped={() => setBootstrapped(true)} />} />
      <Route
        path="/*"
        element={
          bootstrapped
            ? <div style={{ color: 'var(--text)', padding: '2rem' }}>Blackbox — dashboard coming soon</div>
            : <Navigate to="/setup" replace />
        }
      />
    </Routes>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <AppRoutes />
    </BrowserRouter>
  )
}
