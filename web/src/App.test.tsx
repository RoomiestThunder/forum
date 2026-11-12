import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import * as apiModule from './lib/api'
import { useAuth } from './stores/auth'
import { renderWithProviders, routeApi } from './test/utils'
import App from './App'

vi.mock('./lib/api', async () => {
  const actual = await vi.importActual<typeof import('./lib/api')>('./lib/api')
  return { ...actual, api: vi.fn() }
})

const post = {
  id: 1,
  user_id: 2,
  title: 'A post on the feed',
  content: 'body',
  created_at: new Date().toISOString(),
  author: 'alice',
  likes: 0,
  dislikes: 0,
  categories: [],
  tags: [],
  comment_count: 0,
  pinned: false,
  draft: false,
}

beforeEach(() => {
  useAuth.getState().clearSession()
  vi.mocked(apiModule.api).mockImplementation(
    routeApi({ 'GET /posts?page=1': [post], 'GET /categories': [] }) as typeof apiModule.api,
  )
})

afterEach(() => {
  vi.mocked(apiModule.api).mockReset()
  vi.unstubAllGlobals()
})

describe('App routing with lazily loaded pages', () => {
  it('renders the feed, which is not lazy', async () => {
    renderWithProviders(<App />, { route: '/' })

    expect(await screen.findByText('A post on the feed')).toBeInTheDocument()
  })

  it('shows the header at once and the page once its code has arrived', async () => {
    renderWithProviders(<App />, { route: '/forgot-password' })

    // The header belongs to the layout, outside the Suspense boundary, so it is
    // there while the page's chunk is still being fetched.
    expect(screen.getByRole('link', { name: /forum/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /forgot|reset/i })).toBeInTheDocument()
  })

  it('loads a lazy page directly from its URL', async () => {
    renderWithProviders(<App />, { route: '/login' })

    expect(await screen.findByLabelText(/email or username/i)).toBeInTheDocument()
  })

  it('sends a signed-out visitor from a protected page to the login page', async () => {
    renderWithProviders(<App />, { route: '/settings' })

    expect(await screen.findByLabelText(/email or username/i)).toBeInTheDocument()
    expect(screen.queryByText('Profile settings')).not.toBeInTheDocument()
  })

  it('answers an unknown URL with the 404 page', async () => {
    renderWithProviders(<App />, { route: '/no/such/page' })

    expect(await screen.findByRole('heading', { name: '404' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /back to the feed/i })).toHaveAttribute('href', '/')
  })

  it('schedules the post page to be fetched when the browser is idle, and cancels on unmount', () => {
    const request = vi.fn(() => 42)
    const cancel = vi.fn()
    vi.stubGlobal('requestIdleCallback', request)
    vi.stubGlobal('cancelIdleCallback', cancel)

    const { unmount } = renderWithProviders(<App />, { route: '/no/such/page' })

    expect(request).toHaveBeenCalledTimes(1)
    unmount()
    expect(cancel).toHaveBeenCalledWith(42)
  })
})
