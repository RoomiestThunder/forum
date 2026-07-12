import { useAuth } from '../stores/auth'
import type { AuthResponse } from './types'

const BASE = '/api/v1'

export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

// Single-flight refresh: concurrent 401s share one refresh round-trip,
// which matters because the backend rotates the refresh token on use.
let refreshInFlight: Promise<boolean> | null = null

async function refreshSession(): Promise<boolean> {
  refreshInFlight ??= (async () => {
    const { refreshToken } = useAuth.getState()
    if (!refreshToken) return false
    try {
      const res = await fetch(`${BASE}/auth/refresh`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: refreshToken }),
      })
      if (!res.ok) {
        useAuth.getState().clearSession()
        return false
      }
      const data = (await res.json()) as AuthResponse
      useAuth.getState().setSession({
        userId: data.user_id,
        username: data.username,
        accessToken: data.access_token,
        refreshToken: data.refresh_token,
      })
      return true
    } catch {
      return false
    } finally {
      refreshInFlight = null
    }
  })()
  return refreshInFlight
}

async function doFetch(path: string, init: RequestInit): Promise<Response> {
  const { accessToken } = useAuth.getState()
  const headers = new Headers(init.headers)
  if (accessToken) headers.set('Authorization', `Bearer ${accessToken}`)
  if (init.body !== undefined && !(init.body instanceof FormData) && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  return fetch(`${BASE}${path}`, { ...init, headers })
}

/** Perform an API request, transparently refreshing an expired access token. */
export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  let res = await doFetch(path, init)
  if (res.status === 401 && useAuth.getState().refreshToken) {
    if (await refreshSession()) {
      res = await doFetch(path, init)
    }
  }
  if (!res.ok) {
    let message = res.statusText
    try {
      const body = (await res.json()) as { error?: string }
      if (body.error) message = body.error
    } catch {
      // not a JSON error body, keep the status text
    }
    throw new ApiError(res.status, message)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

/** Upload a file as multipart/form-data under the "file" field. */
export async function uploadFile<T>(path: string, file: File): Promise<T> {
  const form = new FormData()
  form.append('file', file)
  return api<T>(path, { method: 'POST', body: form })
}

export async function login(loginValue: string, password: string): Promise<void> {
  const data = await api<AuthResponse>('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ login: loginValue, password }),
  })
  useAuth.getState().setSession({
    userId: data.user_id,
    username: data.username,
    accessToken: data.access_token,
    refreshToken: data.refresh_token,
  })
}

export async function register(email: string, username: string, password: string): Promise<void> {
  const data = await api<AuthResponse>('/auth/register', {
    method: 'POST',
    body: JSON.stringify({ email, username, password }),
  })
  useAuth.getState().setSession({
    userId: data.user_id,
    username: data.username,
    accessToken: data.access_token,
    refreshToken: data.refresh_token,
  })
}

export async function logout(): Promise<void> {
  try {
    await api('/auth/logout', { method: 'POST' })
  } catch {
    // The server-side token cleanup is best effort; drop the local session regardless.
  }
  useAuth.getState().clearSession()
}
