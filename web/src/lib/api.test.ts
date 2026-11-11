import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuth } from '../stores/auth'
import { ApiError, api, login, logout, register, uploadFile } from './api'

type Handler = (url: string, init: RequestInit) => Response | Promise<Response>

/** Replaces fetch with a handler and returns the mock for inspecting calls. */
function mockFetch(handler: Handler) {
  const fn = vi.fn((url: string, init: RequestInit = {}) => Promise.resolve(handler(url, init)))
  vi.stubGlobal('fetch', fn)
  return fn
}

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

const session = (accessToken: string, refreshToken = 'refresh-1') => ({
  userId: 1,
  username: 'alice',
  accessToken,
  refreshToken,
})

const authHeader = (init: RequestInit) => new Headers(init.headers).get('Authorization')

type FetchMock = ReturnType<typeof mockFetch>
const initOf = (fn: FetchMock, call: number): RequestInit => fn.mock.calls[call][1] ?? {}
const bodyOf = (fn: FetchMock, call: number): unknown => JSON.parse(initOf(fn, call).body as string)

const refreshResponse = (access: string, refresh: string) =>
  json({ access_token: access, refresh_token: refresh, user_id: 1, username: 'alice' })

beforeEach(() => {
  useAuth.getState().clearSession()
})

afterEach(() => {
  useAuth.getState().clearSession()
})

describe('api()', () => {
  it('sends the access token when signed in and none when not', async () => {
    const fetchMock = mockFetch(() => json({ ok: true }))

    await api('/anything')
    expect(authHeader(initOf(fetchMock, 0))).toBeNull()

    useAuth.getState().setSession(session('access-1'))
    await api('/anything')
    expect(authHeader(initOf(fetchMock, 1))).toBe('Bearer access-1')
  })

  it('prefixes paths with /api/v1', async () => {
    const fetchMock = mockFetch(() => json({}))
    await api('/posts')
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/posts')
  })

  it('sends a JSON content type for a string body, but lets the browser set one for FormData', async () => {
    const fetchMock = mockFetch(() => json({}))

    await api('/x', { method: 'POST', body: JSON.stringify({ a: 1 }) })
    expect(new Headers(initOf(fetchMock, 0).headers).get('Content-Type')).toBe('application/json')

    await uploadFile('/upload', new File(['x'], 'x.png'))
    // A multipart body needs a boundary only the browser can generate.
    expect(new Headers(initOf(fetchMock, 1).headers).has('Content-Type')).toBe(false)
  })

  it('returns the parsed body, and undefined for 204 No Content', async () => {
    mockFetch((url) => (url.endsWith('/gone') ? new Response(null, { status: 204 }) : json({ id: 7 })))

    expect(await api('/thing')).toEqual({ id: 7 })
    expect(await api('/gone')).toBeUndefined()
  })

  it("throws an ApiError carrying the server's message and status", async () => {
    mockFetch(() => json({ error: "you can't message this user" }, 403))

    const error = await api('/x').catch((e: unknown) => e)

    expect(error).toBeInstanceOf(ApiError)
    expect((error as ApiError).status).toBe(403)
    expect((error as ApiError).message).toBe("you can't message this user")
  })

  it('falls back to the status text when the error body is not JSON', async () => {
    mockFetch(() => new Response('<html>Bad gateway</html>', { status: 502, statusText: 'Bad Gateway' }))

    await expect(api('/x')).rejects.toMatchObject({ status: 502, message: 'Bad Gateway' })
  })
})

describe('transparent token refresh', () => {
  it('refreshes after a 401, stores the new tokens and retries with them', async () => {
    useAuth.getState().setSession(session('expired', 'refresh-1'))
    const fetchMock = mockFetch((url, init) => {
      if (url.endsWith('/auth/refresh')) return refreshResponse('fresh-access', 'refresh-2')
      return authHeader(init) === 'Bearer fresh-access' ? json({ data: 'secret' }) : json({ error: 'expired' }, 401)
    })

    expect(await api('/me')).toEqual({ data: 'secret' })

    expect(fetchMock.mock.calls.map((c) => c[0])).toEqual(['/api/v1/me', '/api/v1/auth/refresh', '/api/v1/me'])
    expect(bodyOf(fetchMock, 1)).toEqual({ refresh_token: 'refresh-1' })
    expect(useAuth.getState().accessToken).toBe('fresh-access')
    // The server rotates refresh tokens, so the new one must replace the old.
    expect(useAuth.getState().refreshToken).toBe('refresh-2')
  })

  it('shares one refresh among concurrent requests', async () => {
    useAuth.getState().setSession(session('expired', 'refresh-1'))
    let refreshes = 0
    mockFetch((url, init) => {
      if (url.endsWith('/auth/refresh')) {
        refreshes += 1
        return refreshResponse('fresh', 'refresh-2')
      }
      return authHeader(init) === 'Bearer fresh' ? json({ ok: true }) : json({ error: 'expired' }, 401)
    })

    // The refresh token is single-use, so a second concurrent refresh would
    // present an already-rotated token and sign the user out.
    const results = await Promise.all([api('/a'), api('/b'), api('/c')])

    expect(results).toEqual([{ ok: true }, { ok: true }, { ok: true }])
    expect(refreshes).toBe(1)
  })

  it('signs the user out when the refresh is refused', async () => {
    useAuth.getState().setSession(session('expired', 'revoked'))
    mockFetch((url) => (url.endsWith('/auth/refresh') ? json({ error: 'invalid refresh token' }, 401) : json({ error: 'expired' }, 401)))

    await expect(api('/me')).rejects.toMatchObject({ status: 401 })

    expect(useAuth.getState().accessToken).toBeNull()
    expect(useAuth.getState().refreshToken).toBeNull()
  })

  it('does not sign the user out when the refresh fails only because the network is down', async () => {
    useAuth.getState().setSession(session('expired', 'refresh-1'))
    mockFetch((url) => {
      if (url.endsWith('/auth/refresh')) throw new TypeError('Failed to fetch')
      return json({ error: 'expired' }, 401)
    })

    await expect(api('/me')).rejects.toMatchObject({ status: 401 })

    expect(useAuth.getState().refreshToken).toBe('refresh-1')
  })

  it('gives up after one refresh instead of looping when the retry is still 401', async () => {
    useAuth.getState().setSession(session('expired', 'refresh-1'))
    let refreshes = 0
    const fetchMock = mockFetch((url) => {
      if (url.endsWith('/auth/refresh')) {
        refreshes += 1
        return refreshResponse('still-bad', 'refresh-2')
      }
      return json({ error: 'nope' }, 401)
    })

    await expect(api('/me')).rejects.toMatchObject({ status: 401 })

    expect(refreshes).toBe(1)
    expect(fetchMock).toHaveBeenCalledTimes(3) // original, refresh, one retry
  })

  it('does not try to refresh when there is no refresh token', async () => {
    const fetchMock = mockFetch(() => json({ error: 'authorization header required' }, 401))

    await expect(api('/me')).rejects.toMatchObject({ status: 401 })

    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('does not refresh for errors other than 401', async () => {
    useAuth.getState().setSession(session('access'))
    const fetchMock = mockFetch(() => json({ error: 'forbidden' }, 403))

    await expect(api('/x')).rejects.toMatchObject({ status: 403 })

    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})

describe('session helpers', () => {
  it('login() posts the credentials and stores the session', async () => {
    const fetchMock = mockFetch(() => json({ access_token: 'a', refresh_token: 'r', user_id: 5, username: 'alice' }))

    await login('alice', 'password123')

    expect(bodyOf(fetchMock, 0)).toEqual({ login: 'alice', password: 'password123' })
    expect(useAuth.getState()).toMatchObject({ userId: 5, username: 'alice', accessToken: 'a', refreshToken: 'r' })
  })

  it('register() posts the details and stores the session', async () => {
    const fetchMock = mockFetch(() => json({ access_token: 'a', refresh_token: 'r', user_id: 9, username: 'bob' }, 201))

    await register('bob@example.com', 'bob', 'password123')

    expect(bodyOf(fetchMock, 0)).toEqual({
      email: 'bob@example.com',
      username: 'bob',
      password: 'password123',
    })
    expect(useAuth.getState().userId).toBe(9)
  })

  it('a failed login stores nothing and surfaces the message', async () => {
    mockFetch(() => json({ error: 'invalid credentials' }, 401))

    await expect(login('alice', 'wrong')).rejects.toThrow('invalid credentials')
    expect(useAuth.getState().accessToken).toBeNull()
  })

  it('logout() clears the session even if the server call fails', async () => {
    useAuth.getState().setSession(session('access'))
    mockFetch(() => json({ error: 'boom' }, 500))

    await logout()

    expect(useAuth.getState().accessToken).toBeNull()
    expect(useAuth.getState().refreshToken).toBeNull()
  })
})
