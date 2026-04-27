import { createContext, useContext } from 'react'
import { useWebSocket } from '../hooks/useWebSocket'
import type { UseWebSocketResult } from '../hooks/useWebSocket'
import { isDemoModeEnabled } from '../demoMode'

const wsUrl = `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}/api/ws`
const demoMode = isDemoModeEnabled(import.meta.env.VITE_DEMO_MODE)

const WebSocketContext = createContext<UseWebSocketResult>({
  status: 'connecting',
  lastMessage: null,
  lastConnectedAt: null,
  reconnect: () => {},
})

// eslint-disable-next-line react-refresh/only-export-components
export function useWebSocketContext() {
  return useContext(WebSocketContext)
}

export function WebSocketProvider({ children }: { children: React.ReactNode }) {
  if (demoMode) {
    return (
      <WebSocketContext.Provider
        value={{
          status: 'disconnected',
          lastMessage: null,
          lastConnectedAt: null,
          reconnect: () => {},
        }}
      >
        {children}
      </WebSocketContext.Provider>
    )
  }

  const ws = useWebSocket(wsUrl)
  return <WebSocketContext.Provider value={ws}>{children}</WebSocketContext.Provider>
}
