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

const DEMO_WS_VALUE: UseWebSocketResult = {
  status: 'disconnected',
  lastMessage: null,
  lastConnectedAt: null,
  reconnect: () => {},
}

function DemoWebSocketProvider({ children }: { children: React.ReactNode }) {
  return <WebSocketContext.Provider value={DEMO_WS_VALUE}>{children}</WebSocketContext.Provider>
}

function LiveWebSocketProvider({ children }: { children: React.ReactNode }) {
  const ws = useWebSocket(wsUrl)
  return <WebSocketContext.Provider value={ws}>{children}</WebSocketContext.Provider>
}

export function WebSocketProvider({ children }: { children: React.ReactNode }) {
  if (demoMode) return <DemoWebSocketProvider>{children}</DemoWebSocketProvider>
  return <LiveWebSocketProvider>{children}</LiveWebSocketProvider>
}
