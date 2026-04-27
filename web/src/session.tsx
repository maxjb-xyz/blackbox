import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { fetchCurrentUser, logout as logoutRequest, type SessionUser } from './api/client'

interface SessionContextValue {
  user: SessionUser | null
  loading: boolean
  updateSession: (nextUser: SessionUser | null) => void
  refreshSession: () => Promise<SessionUser | null>
  logout: () => Promise<void>
}

const SessionContext = createContext<SessionContextValue | null>(null)

export function SessionProvider({
  children,
  fixedUser,
}: {
  children: ReactNode
  fixedUser?: SessionUser | null
}) {
  const isFixedSession = fixedUser !== undefined
  const [user, setUser] = useState<SessionUser | null>(fixedUser ?? null)
  const [loading, setLoading] = useState(!isFixedSession)
  const requestSeqRef = useRef(0)

  const updateSession = useCallback((nextUser: SessionUser | null) => {
    setUser(isFixedSession ? (fixedUser ?? nextUser) : nextUser)
    setLoading(false)
  }, [fixedUser, isFixedSession])

  const refreshSession = useCallback(async () => {
    if (isFixedSession) {
      setUser(fixedUser ?? null)
      setLoading(false)
      return fixedUser ?? null
    }
    const requestID = requestSeqRef.current + 1
    requestSeqRef.current = requestID
    try {
      const nextUser = await fetchCurrentUser()
      if (requestSeqRef.current !== requestID) return null
      setUser(nextUser)
      setLoading(false)
      return nextUser
    } catch {
      if (requestSeqRef.current !== requestID) return null
      setUser(null)
      setLoading(false)
      return null
    }
  }, [fixedUser, isFixedSession])

  const logout = useCallback(async () => {
    if (isFixedSession) {
      setUser(fixedUser ?? null)
      setLoading(false)
      return
    }
    await logoutRequest()
    updateSession(null)
  }, [fixedUser, isFixedSession, updateSession])

  useEffect(() => {
    if (isFixedSession) {
      setUser(fixedUser ?? null)
      setLoading(false)
      return
    }
    void refreshSession()
  }, [fixedUser, isFixedSession, refreshSession])

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
