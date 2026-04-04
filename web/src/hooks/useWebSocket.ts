import { useCallback, useEffect, useRef, useState } from 'react'

export type WSStatus = 'connecting' | 'connected' | 'disconnected'

export interface WSMessage {
  type: string
  data: unknown
}

export interface UseWebSocketResult {
  status: WSStatus
  lastMessage: WSMessage | null
  lastConnectedAt: Date | null
  reconnect: () => void
}

const BACKOFF_INITIAL = 1000
const BACKOFF_MAX = 30000

export function useWebSocket(url: string): UseWebSocketResult {
  const [status, setStatus] = useState<WSStatus>('connecting')
  const [lastMessage, setLastMessage] = useState<WSMessage | null>(null)
  const [lastConnectedAt, setLastConnectedAt] = useState<Date | null>(null)

  const wsRef = useRef<WebSocket | null>(null)
  const backoffRef = useRef(BACKOFF_INITIAL)
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const mountedRef = useRef(true)
  const manualReconnectRef = useRef(false)

  const connect = useCallback(() => {
    if (!mountedRef.current) return
    if (wsRef.current) {
      wsRef.current.onclose = null
      wsRef.current.close()
    }

    setStatus('connecting')
    const ws = new WebSocket(url)
    wsRef.current = ws

    ws.onopen = () => {
      if (!mountedRef.current) return
      setStatus('connected')
      setLastConnectedAt(new Date())
      backoffRef.current = BACKOFF_INITIAL
      manualReconnectRef.current = false
    }

    ws.onmessage = (event: MessageEvent<string>) => {
      if (!mountedRef.current) return
      try {
        const msg = JSON.parse(event.data) as WSMessage
        setLastMessage(msg)
      } catch {
        // ignore malformed messages
      }
    }

    ws.onclose = () => {
      if (!mountedRef.current) return
      wsRef.current = null
      setStatus('disconnected')
      if (!manualReconnectRef.current) {
        retryTimerRef.current = setTimeout(() => {
          backoffRef.current = Math.min(backoffRef.current * 2, BACKOFF_MAX)
          connect()
        }, backoffRef.current)
      }
    }

    ws.onerror = () => {
      ws.close()
    }
  }, [url])

  const reconnect = useCallback(() => {
    if (retryTimerRef.current) {
      clearTimeout(retryTimerRef.current)
      retryTimerRef.current = null
    }
    backoffRef.current = BACKOFF_INITIAL
    manualReconnectRef.current = true
    connect()
  }, [connect])

  useEffect(() => {
    mountedRef.current = true
    connect()
    return () => {
      mountedRef.current = false
      if (retryTimerRef.current) clearTimeout(retryTimerRef.current)
      if (wsRef.current) {
        wsRef.current.onclose = null
        wsRef.current.close()
      }
    }
  }, [connect])

  return { status, lastMessage, lastConnectedAt, reconnect }
}
