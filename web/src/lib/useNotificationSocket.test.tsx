import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useAuth } from '../stores/auth'
import { useNotificationSocket } from './useNotificationSocket'

class FakeSocket {
  static instances: FakeSocket[] = []

  url: string
  sent: string[] = []
  closed = false
  onopen: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  onclose: (() => void) | null = null

  constructor(url: string) {
    this.url = url
    FakeSocket.instances.push(this)
  }

  send(data: string) {
    this.sent.push(data)
  }

  close() {
    this.closed = true
  }

  // Test helpers simulating what the server / network does.
  open() {
    this.onopen?.()
  }
  receive(frame: unknown) {
    this.onmessage?.({ data: JSON.stringify(frame) })
  }
  drop() {
    this.onclose?.()
  }
}

const signIn = (userId: number, accessToken: string) =>
  useAuth.setState({ userId, username: `user${userId}`, accessToken, refreshToken: 'refresh' })
const signOut = () =>
  useAuth.setState({ userId: null, username: null, accessToken: null, refreshToken: null })

function setup() {
  const queryClient = new QueryClient()
  const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  const view = renderHook(() => useNotificationSocket(), { wrapper })
  return { ...view, invalidate }
}

const invalidatedKeys = (spy: { mock: { calls: unknown[][] } }) =>
  spy.mock.calls.map((call) => (call[0] as { queryKey: string[] }).queryKey[0])

describe('useNotificationSocket', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    FakeSocket.instances = []
    vi.stubGlobal('WebSocket', FakeSocket as unknown as typeof WebSocket)
    signOut()
  })

  afterEach(() => {
    vi.useRealTimers()
    signOut()
  })

  it('does not connect while signed out', () => {
    setup()
    expect(FakeSocket.instances).toHaveLength(0)
  })

  it('authenticates with its first frame and keeps the token out of the URL', () => {
    signIn(1, 'secret-token')
    setup()

    const [socket] = FakeSocket.instances
    expect(socket.url).toMatch(/\/api\/v1\/ws$/)
    expect(socket.url).not.toContain('secret-token')
    expect(socket.url).not.toContain('token=')

    act(() => socket.open())
    expect(socket.sent).toEqual([JSON.stringify({ type: 'auth', token: 'secret-token' })])
  })

  it('refetches notifications on a notification event', () => {
    signIn(1, 'tok')
    const { invalidate } = setup()

    act(() => FakeSocket.instances[0].receive({ type: 'new_comment' }))

    expect(invalidatedKeys(invalidate)).toEqual(['notifications'])
  })

  it('refetches conversations and messages on a new_message event', () => {
    signIn(1, 'tok')
    const { invalidate } = setup()

    act(() => FakeSocket.instances[0].receive({ type: 'new_message' }))

    expect(invalidatedKeys(invalidate)).toEqual(['conversations', 'messages'])
  })

  it('does not refetch anything for the ready frame', () => {
    signIn(1, 'tok')
    const { invalidate } = setup()

    act(() => FakeSocket.instances[0].receive({ type: 'ready' }))

    expect(invalidate).not.toHaveBeenCalled()
  })

  it('falls back to refreshing notifications for a malformed frame', () => {
    signIn(1, 'tok')
    const { invalidate } = setup()

    act(() => FakeSocket.instances[0].onmessage?.({ data: 'not json' }))

    expect(invalidatedKeys(invalidate)).toEqual(['notifications'])
  })

  it('keeps the same socket when the access token rotates', () => {
    signIn(1, 'old-token')
    setup()

    act(() => signIn(1, 'new-token'))

    expect(FakeSocket.instances).toHaveLength(1)
    expect(FakeSocket.instances[0].closed).toBe(false)
  })

  it('reconnects with exponential backoff, using the current token', () => {
    signIn(1, 'old-token')
    setup()

    act(() => FakeSocket.instances[0].drop())
    act(() => vi.advanceTimersByTime(2_999))
    expect(FakeSocket.instances).toHaveLength(1) // first retry waits 3s

    signIn(1, 'refreshed-token') // refreshed while we were disconnected
    act(() => vi.advanceTimersByTime(1))
    expect(FakeSocket.instances).toHaveLength(2)
    act(() => FakeSocket.instances[1].open())
    expect(FakeSocket.instances[1].sent[0]).toContain('refreshed-token')

    act(() => FakeSocket.instances[1].drop()) // dropped again without a ready
    act(() => vi.advanceTimersByTime(5_999))
    expect(FakeSocket.instances).toHaveLength(2) // second retry waits 6s
    act(() => vi.advanceTimersByTime(1))
    expect(FakeSocket.instances).toHaveLength(3)
  })

  it('resets the backoff once a connection is accepted', () => {
    signIn(1, 'tok')
    setup()

    act(() => FakeSocket.instances[0].drop())
    act(() => vi.advanceTimersByTime(3_000))
    act(() => FakeSocket.instances[1].receive({ type: 'ready' }))
    act(() => FakeSocket.instances[1].drop())

    act(() => vi.advanceTimersByTime(3_000))
    expect(FakeSocket.instances).toHaveLength(3) // back to the 3s base delay
  })

  it('closes the socket and stops reconnecting on sign out', () => {
    signIn(1, 'tok')
    setup()
    const [socket] = FakeSocket.instances

    act(() => signOut())
    expect(socket.closed).toBe(true)

    act(() => socket.drop())
    act(() => vi.advanceTimersByTime(120_000))
    expect(FakeSocket.instances).toHaveLength(1)
  })

  it('reconnects as the new user when the account switches', () => {
    signIn(1, 'alice-token')
    setup()

    act(() => signIn(2, 'bob-token'))

    expect(FakeSocket.instances).toHaveLength(2)
    expect(FakeSocket.instances[0].closed).toBe(true)
    act(() => FakeSocket.instances[1].open())
    expect(FakeSocket.instances[1].sent[0]).toContain('bob-token')
  })
})
