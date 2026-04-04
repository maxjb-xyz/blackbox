import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import type { Node } from '../api/client'
import { fetchNodes } from '../api/client'
import { useWebSocketContext } from './WebSocketProvider'

interface NodePulseContextValue {
  nodes: Node[]
  onlineCount: number
  totalCount: number
  loading: boolean
  error: Error | null
  lastUpdated: Date | null
}

const NodePulseContext = createContext<NodePulseContextValue>({
  nodes: [],
  onlineCount: 0,
  totalCount: 0,
  loading: true,
  error: null,
  lastUpdated: null,
})

// eslint-disable-next-line react-refresh/only-export-components
export function useNodePulse() {
  return useContext(NodePulseContext)
}

export function NodePulseProvider({ children }: { children: React.ReactNode }) {
  const [nodes, setNodes] = useState<Node[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<Error | null>(null)
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null)
  const pollingRef = useRef(false)
  const queuedRef = useRef(false)
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    return () => { mountedRef.current = false }
  }, [])

  const poll = useCallback(async () => {
    if (!mountedRef.current) return
    if (pollingRef.current) {
      queuedRef.current = true
      return
    }

    pollingRef.current = true
    if (mountedRef.current) setLoading(true)
    try {
      const data = await fetchNodes()
      if (!mountedRef.current) return
      setNodes(data)
      setError(null)
      setLastUpdated(new Date())
    } catch (err) {
      if (!mountedRef.current) return
      setError(err instanceof Error ? err : new Error('Failed to fetch nodes'))
    } finally {
      pollingRef.current = false
      if (mountedRef.current) setLoading(false)
      if (queuedRef.current) {
        queuedRef.current = false
        void poll()
      }
    }
  }, [])

  useEffect(() => {
    void poll()
    const interval = setInterval(() => void poll(), 30_000)
    return () => clearInterval(interval)
  }, [poll])

  // Trigger immediate refresh on WS node_status message
  const { lastMessage } = useWebSocketContext()
  useEffect(() => {
    if (!lastMessage || lastMessage.type !== 'node_status') return
    void poll()
  }, [lastMessage, poll])

  const onlineCount = nodes.filter(node => node.status === 'online').length
  const totalCount = nodes.length

  return (
    <NodePulseContext.Provider value={{ nodes, onlineCount, totalCount, loading, error, lastUpdated }}>
      {children}
    </NodePulseContext.Provider>
  )
}
