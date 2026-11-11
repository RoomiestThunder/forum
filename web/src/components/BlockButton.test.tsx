import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as apiModule from '../lib/api'
import { useAuth } from '../stores/auth'
import { renderWithProviders, routeApi } from '../test/utils'
import BlockButton from './BlockButton'
import BlockedUsers from './BlockedUsers'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return { ...actual, api: vi.fn() }
})

const bob = { id: 2, username: 'bob', avatar_url: '' }

beforeEach(() => {
  useAuth.getState().setSession({ userId: 1, username: 'alice', accessToken: 't', refreshToken: 'r' })
})

afterEach(() => {
  vi.mocked(apiModule.api).mockReset()
  useAuth.getState().clearSession()
})

describe('BlockButton', () => {
  it('offers to block someone who is not blocked, and blocks them on click', async () => {
    const block = vi.fn().mockReturnValue({ status: 'ok' })
    vi.mocked(apiModule.api).mockImplementation(
      routeApi({ 'GET /blocks': [], 'PUT /blocks/bob': block }) as typeof apiModule.api,
    )
    renderWithProviders(<BlockButton username="bob" />)
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: 'Block' }))

    await waitFor(() => expect(block).toHaveBeenCalledTimes(1))
  })

  it('offers to unblock someone already blocked, and unblocks on click', async () => {
    const unblock = vi.fn().mockReturnValue({ status: 'ok' })
    vi.mocked(apiModule.api).mockImplementation(
      routeApi({ 'GET /blocks': [bob], 'DELETE /blocks/bob': unblock }) as typeof apiModule.api,
    )
    renderWithProviders(<BlockButton username="bob" />)
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: 'Unblock' }))

    await waitFor(() => expect(unblock).toHaveBeenCalledTimes(1))
  })

  it('matches usernames case-insensitively, like the server does', async () => {
    vi.mocked(apiModule.api).mockImplementation(routeApi({ 'GET /blocks': [bob] }) as typeof apiModule.api)
    renderWithProviders(<BlockButton username="BOB" />)

    expect(await screen.findByRole('button', { name: 'Unblock' })).toBeInTheDocument()
  })
})

describe('BlockedUsers', () => {
  it('lists blocked users with links to their profiles', async () => {
    vi.mocked(apiModule.api).mockImplementation(routeApi({ 'GET /blocks': [bob] }) as typeof apiModule.api)
    renderWithProviders(<BlockedUsers />)

    expect(await screen.findByRole('link', { name: 'bob' })).toHaveAttribute('href', '/u/bob')
  })

  it('says so when nobody is blocked', async () => {
    vi.mocked(apiModule.api).mockImplementation(routeApi({ 'GET /blocks': [] }) as typeof apiModule.api)
    renderWithProviders(<BlockedUsers />)

    expect(await screen.findByText("You haven't blocked anyone.")).toBeInTheDocument()
  })

  it('unblocks from the list', async () => {
    const unblock = vi.fn().mockReturnValue({ status: 'ok' })
    vi.mocked(apiModule.api).mockImplementation(
      routeApi({ 'GET /blocks': [bob], 'DELETE /blocks/bob': unblock }) as typeof apiModule.api,
    )
    renderWithProviders(<BlockedUsers />)
    const user = userEvent.setup()

    await user.click(await screen.findByRole('button', { name: 'Unblock bob' }))

    await waitFor(() => expect(unblock).toHaveBeenCalledTimes(1))
  })
})
