import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { fetchCurrentUser, logout as logoutRequest, type SessionUser } from './api/client'

interface SessionContextValue {
  user: SessionUser | null
  loading: boolean
  updateSession: (nextUser: SessionUser | null) => void
  refreshSession: () => Promise<SessionUser | null>
  logout: () => Promise<void>
}

const SessionContext = createContext<SessionContextValue | null>(null)

export function SessionProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<SessionUser | null>(null)
  const [loading, setLoading] = useState(true)

  const updateSession = useCallback((nextUser: SessionUser | null) => {
    setUser(nextUser)
    setLoading(false)
  }, [])

  const refreshSession = useCallback(async () => {
    try {
      const nextUser = await fetchCurrentUser()
      updateSession(nextUser)
      return nextUser
    } catch {
      updateSession(null)
      return null
    }
  }, [updateSession])

  const logout = useCallback(async () => {
    try {
      await logoutRequest()
    } finally {
      updateSession(null)
    }
  }, [updateSession])

  useEffect(() => {
    void refreshSession()
  }, [refreshSession])

  const value = useMemo(
    () => ({
      user,
      loading,
      updateSession,
      refreshSession,
      logout,
    }),
    [user, loading, updateSession, refreshSession, logout],
  )

  return (
    <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export function useSession() {
  const context = useContext(SessionContext)
  if (!context) {
    throw new Error('useSession must be used within SessionProvider')
  }
  return context
}
