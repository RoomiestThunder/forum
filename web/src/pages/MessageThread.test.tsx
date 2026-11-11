import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router'
import * as apiModule from '../lib/api'
import { ApiError } from '../lib/api'
import { useAuth } from '../stores/auth'
import { routeApi } from '../test/utils'
import MessageThread from './MessageThread'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return { ...actual, api: vi.fn() }
})

// The page reads :username from the route, so it has to be rendered under one.
function renderThread(username = 'bob') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[`/messages/${username}`]}>
        <Routes>
          <Route path="/messages/:username" element={<MessageThread />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  // jsdom doesn't implement scrollIntoView, which the page calls on mount.
  Element.prototype.scrollIntoView = vi.fn()
  useAuth.getState().setSession({ userId: 1, username: 'alice', accessToken: 't', refreshToken: 'r' })
})

afterEach(() => {
  vi.mocked(apiModule.api).mockReset()
  useAuth.getState().clearSession()
})

describe('MessageThread', () => {
  it("shows the server's reason when a message is refused", async () => {
    vi.mocked(apiModule.api).mockImplementation(
      routeApi({
        'GET /blocks': [],
        'GET /messages/bob': { messages: [] },
        'POST /messages/bob': () => {
          throw new ApiError(403, "you can't message this user")
        },
      }) as typeof apiModule.api,
    )
    renderThread()
    const user = userEvent.setup()

    await user.type(await screen.findByPlaceholderText('Write a message…'), 'hello')
    await user.click(screen.getByRole('button', { name: 'Send' }))

    expect(await screen.findByRole('alert')).toHaveTextContent("you can't message this user")
  })

  it('disables the composer and explains why when you blocked the other user', async () => {
    vi.mocked(apiModule.api).mockImplementation(
      routeApi({
        'GET /blocks': [{ id: 2, username: 'bob', avatar_url: '' }],
        'GET /messages/bob': { messages: [] },
      }) as typeof apiModule.api,
    )
    renderThread()

    expect(await screen.findByText(/You blocked bob/)).toBeInTheDocument()
    expect(screen.getByPlaceholderText('Write a message…')).toBeDisabled()
  })

  it('leaves the composer enabled when nobody is blocked', async () => {
    vi.mocked(apiModule.api).mockImplementation(
      routeApi({ 'GET /blocks': [], 'GET /messages/bob': { messages: [] } }) as typeof apiModule.api,
    )
    renderThread()

    expect(await screen.findByPlaceholderText('Write a message…')).toBeEnabled()
    expect(screen.queryByText(/You blocked/)).not.toBeInTheDocument()
  })
})
