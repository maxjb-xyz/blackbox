import { createContext, useContext } from 'react'
import { useWebSocket } from '../hooks/useWebSocket'
import type { UseWebSocketResult } from '../hooks/useWebSocket'

const wsUrl = `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}/api/ws`

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
  const ws = useWebSocket(wsUrl)
  return <WebSocketContext.Provider value={ws}>{children}</WebSocketContext.Provider>
}
