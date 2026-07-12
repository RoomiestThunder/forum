import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useAuth } from '../stores/auth'

/**
 * Opens a websocket to the notification hub and, on any message, invalidates
 * the notification queries so the bell refetches. The message payload is
 * intentionally not trusted as the source of truth (see the notify() helper
 * server-side) - a socket message just means "something happened, go check."
 */
export function useNotificationSocket() {
  const accessToken = useAuth((s) => s.accessToken)
  const qc = useQueryClient()

  useEffect(() => {
    if (!accessToken) return

    let socket: WebSocket | null = null
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let closedByEffect = false

    const connect = () => {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      socket = new WebSocket(`${protocol}//${window.location.host}/api/v1/ws?token=${accessToken}`)

      socket.onmessage = () => {
        void qc.invalidateQueries({ queryKey: ['notifications'] })
      }

      socket.onclose = () => {
        if (closedByEffect) return
        reconnectTimer = setTimeout(connect, 3000)
      }
    }

    connect()

    return () => {
      closedByEffect = true
      if (reconnectTimer) clearTimeout(reconnectTimer)
      socket?.close()
    }
  }, [accessToken, qc])
}
