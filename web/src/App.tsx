import { useEffect, useState } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { checkSetupStatus } from './api/client'
import Shell from './components/Shell'
import AccountPage from './pages/AccountPage'
import AdminPage from './pages/AdminPage'
import DiagnosticsPage from './pages/DiagnosticsPage'
import LoginPage from './pages/LoginPage'
import NodesPage from './pages/NodesPage'
import SetupPage from './pages/SetupPage'
import TimelinePage from './pages/TimelinePage'
import WebhooksPage from './pages/WebhooksPage'

function AppRoutes() {
  const [bootstrapped, setBootstrapped] = useState<boolean | null>(null)

  useEffect(() => {
    checkSetupStatus()
      .then(status => setBootstrapped(status.bootstrapped))
      .catch(() => setBootstrapped(false))
  }, [])

  if (bootstrapped === null) {
    return (
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          minHeight: '100vh',
          color: 'var(--muted)',
          fontSize: '12px',
        }}
      >
        loading...
      </div>
    )
  }

  if (!bootstrapped) {
    return (
      <Routes>
        <Route path="*" element={<Navigate to="/setup" replace />} />
        <Route path="/setup" element={<SetupPage onBootstrapped={() => setBootstrapped(true)} />} />
      </Routes>
    )
  }

  return (
    <Routes>
      <Route path="/setup" element={<Navigate to="/" replace />} />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/" element={<Shell />}>
        <Route index element={<Navigate to="/timeline" replace />} />
        <Route path="timeline" element={<TimelinePage />} />
        <Route path="nodes" element={<NodesPage />} />
        <Route path="webhooks" element={<WebhooksPage />} />
        <Route path="account" element={<AccountPage />} />
        <Route path="admin" element={<AdminPage />} />
        <Route path="diagnostics" element={<DiagnosticsPage />} />
      </Route>
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
