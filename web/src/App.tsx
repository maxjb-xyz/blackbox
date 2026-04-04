import { useEffect, useState } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { checkSetupStatus } from './api/client'
import Shell from './components/Shell'
import AccountPage from './pages/AccountPage'
import AdminPage from './pages/AdminPage'
import DiagnosticsPage from './pages/DiagnosticsPage'
import LoginPage from './pages/LoginPage'
import NodesPage from './pages/NodesPage'
import RegisterPage from './pages/RegisterPage'
import SetupPage from './pages/SetupPage'
import TimelinePage from './pages/TimelinePage'
import WebhooksPage from './pages/WebhooksPage'
import { SessionProvider } from './session'

function AppRoutes() {
  const [bootstrapped, setBootstrapped] = useState<boolean | null>(null)
  const [checkingSetup, setCheckingSetup] = useState(true)
  const [setupError, setSetupError] = useState<string | null>(null)
  const [setupCheckAttempt, setSetupCheckAttempt] = useState(0)

  useEffect(() => {
    let cancelled = false

    checkSetupStatus()
      .then(status => {
        if (cancelled) return
        setBootstrapped(status.bootstrapped)
      })
      .catch(err => {
        if (cancelled) return
        setSetupError(err instanceof Error ? err.message : 'Failed to check setup status')
      })
      .finally(() => {
        if (!cancelled) setCheckingSetup(false)
      })

    return () => {
      cancelled = true
    }
  }, [setupCheckAttempt])

  if (bootstrapped === null && checkingSetup) {
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

  if (bootstrapped === null && setupError) {
    return (
      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          gap: 12,
          minHeight: '100vh',
          color: 'var(--muted)',
          fontSize: '12px',
        }}
      >
        <span>{setupError}</span>
        <button
          type="button"
          onClick={() => {
            setCheckingSetup(true)
            setSetupError(null)
            setSetupCheckAttempt(attempt => attempt + 1)
          }}
          style={{
            background: 'transparent',
            border: '1px solid var(--border)',
            color: 'var(--text)',
            padding: '8px 12px',
            fontFamily: 'inherit',
            fontSize: '12px',
            cursor: 'pointer',
            letterSpacing: '0.08em',
          }}
        >
          RETRY
        </button>
      </div>
    )
  }

  if (!bootstrapped) {
    return (
      <Routes>
        <Route path="*" element={<Navigate to="/setup" replace />} />
        <Route
          path="/setup"
          element={
            <SetupPage
              onBootstrapped={() => {
                setSetupError(null)
                setBootstrapped(true)
              }}
            />
          }
        />
      </Routes>
    )
  }

  return (
    <Routes>
      <Route path="/setup" element={<Navigate to="/" replace />} />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route path="/" element={<Shell />}>
        <Route index element={<Navigate to="/timeline" replace />} />
        <Route path="timeline" element={<TimelinePage />} />
        <Route path="nodes" element={<NodesPage />} />
        <Route path="webhooks" element={<WebhooksPage />} />
        <Route path="account" element={<AccountPage />} />
        <Route path="admin" element={<AdminPage />} />
        <Route path="diagnostics" element={<DiagnosticsPage />} />
        <Route path="*" element={<Navigate to="/timeline" replace />} />
      </Route>
    </Routes>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <SessionProvider>
        <AppRoutes />
      </SessionProvider>
    </BrowserRouter>
  )
}
