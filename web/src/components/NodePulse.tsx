import { createContext, useContext, useEffect, useRef, useState } from 'react'
import type { Node } from '../api/client'
import { fetchNodes } from '../api/client'

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

  useEffect(() => {
    let cancelled = false

    async function poll() {
      if (cancelled || pollingRef.current) return

      pollingRef.current = true
      if (!cancelled) setLoading(true)
      try {
        const data = await fetchNodes()
        if (cancelled) return
        setNodes(data)
        setError(null)
        setLastUpdated(new Date())
      } catch (err) {
        if (cancelled) return
        setError(err instanceof Error ? err : new Error('Failed to fetch nodes'))
      } finally {
        pollingRef.current = false
        if (!cancelled) setLoading(false)
      }
    }

    void poll()
    const interval = setInterval(() => {
      void poll()
    }, 30_000)
    return () => {
      cancelled = true
      pollingRef.current = false
      clearInterval(interval)
    }
  }, [])

  const onlineCount = nodes.filter(node => node.status === 'online').length
  const totalCount = nodes.length

  return (
    <NodePulseContext.Provider value={{ nodes, onlineCount, totalCount, loading, error, lastUpdated }}>
      {children}
    </NodePulseContext.Provider>
  )
}
