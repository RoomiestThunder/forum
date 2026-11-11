import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useAuth } from '../stores/auth'

const RECONNECT_BASE_MS = 3_000
const RECONNECT_MAX_MS = 60_000

/**
 * Opens a websocket to the notification hub and, on any event, invalidates
 * the relevant queries so the UI refetches. The event payload is
 * intentionally not trusted as the source of truth (see the notify() helper
 * server-side) - an event just means "something happened, go check."
 *
 * The access token is sent as the first frame rather than in the URL, which
 * would leak it into proxy and access logs. The connection is keyed on the
 * signed-in user, not on the token: the token rotates every few minutes, and
 * tearing the socket down on each rotation would be pointless churn. Every
 * (re)connect reads the current token from the store instead.
 */
export function useNotificationSocket() {
  const userId = useAuth((s) => s.userId)
  const qc = useQueryClient()

  useEffect(() => {
    if (userId === null) return

    let socket: WebSocket | null = null
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let closedByEffect = false
    let failures = 0

    const connect = () => {
      const token = useAuth.getState().accessToken
      if (!token) return

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const ws = new WebSocket(`${protocol}//${window.location.host}/api/v1/ws`)
      socket = ws

      ws.onopen = () => {
        ws.send(JSON.stringify({ type: 'auth', token }))
      }

      ws.onmessage = (event) => {
        let type: string | undefined
        try {
          type = (JSON.parse(event.data as string) as { type?: string }).type
        } catch {
          // Malformed payload - fall through to the generic refresh below.
        }
        if (type === 'ready') {
          // The server accepted the token: the connection is healthy, so the
          // next drop starts the backoff over.
          failures = 0
        } else if (type === 'new_message') {
          void qc.invalidateQueries({ queryKey: ['conversations'] })
          void qc.invalidateQueries({ queryKey: ['messages'] })
        } else {
          void qc.invalidateQueries({ queryKey: ['notifications'] })
        }
      }

      ws.onclose = () => {
        if (closedByEffect) return
        // Exponential backoff, so a server that rejects us (expired token,
        // suspended account) or is down isn't hammered every few seconds.
        const delay = Math.min(RECONNECT_BASE_MS * 2 ** failures, RECONNECT_MAX_MS)
        failures += 1
        reconnectTimer = setTimeout(connect, delay)
      }
    }

    connect()

    return () => {
      closedByEffect = true
      if (reconnectTimer) clearTimeout(reconnectTimer)
      socket?.close()
    }
  }, [userId, qc])
}
