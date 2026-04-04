import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { fetchCurrentUser, logout as logoutRequest, type SessionUser } from './api/client'

interface SessionContextValue {
  user: SessionUser | null
  loading: boolean
  refreshSession: () => Promise<SessionUser | null>
  logout: () => Promise<void>
}

const SessionContext = createContext<SessionContextValue | null>(null)

export function SessionProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<SessionUser | null>(null)
  const [loading, setLoading] = useState(true)

  async function refreshSession() {
    try {
      const nextUser = await fetchCurrentUser()
      setUser(nextUser)
      return nextUser
    } catch {
      setUser(null)
      return null
    } finally {
      setLoading(false)
    }
  }

  async function logout() {
    try {
      await logoutRequest()
    } finally {
      setUser(null)
      setLoading(false)
    }
  }

  useEffect(() => {
    void refreshSession()
  }, [])

  return (
    <SessionContext.Provider
      value={{
        user,
        loading,
        refreshSession,
        logout,
      }}
    >
      {children}
    </SessionContext.Provider>
  )
}

export function useSession() {
  const context = useContext(SessionContext)
  if (!context) {
    throw new Error('useSession must be used within SessionProvider')
  }
  return context
}
