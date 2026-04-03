import { createContext, useContext, useEffect, useState } from 'react'
import type { Node } from '../api/client'
import { fetchNodes } from '../api/client'

interface NodePulseContextValue {
  nodes: Node[]
  onlineCount: number
  totalCount: number
}

const NodePulseContext = createContext<NodePulseContextValue>({
  nodes: [],
  onlineCount: 0,
  totalCount: 0,
})

export function useNodePulse() {
  return useContext(NodePulseContext)
}

export function NodePulseProvider({ children }: { children: React.ReactNode }) {
  const [nodes, setNodes] = useState<Node[]>([])

  useEffect(() => {
    let cancelled = false

    function poll() {
      fetchNodes()
        .then(data => {
          if (!cancelled) setNodes(data)
        })
        .catch(() => {
          /* silent fail - sidebar degrades gracefully */
        })
    }

    poll()
    const interval = setInterval(poll, 30_000)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [])

  const onlineCount = nodes.filter(node => node.status === 'online').length
  const totalCount = nodes.length

  return (
    <NodePulseContext.Provider value={{ nodes, onlineCount, totalCount }}>
      {children}
    </NodePulseContext.Provider>
  )
}
