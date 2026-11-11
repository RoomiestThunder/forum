import { beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '../test/utils'
import * as api from '../lib/api'
import Register from './Register'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return { ...actual, register: vi.fn() }
})

describe('Register', () => {
  beforeEach(() => {
    vi.mocked(api.register).mockReset()
  })

  it('registers with valid input', async () => {
    vi.mocked(api.register).mockResolvedValue(undefined)
    renderWithProviders(<Register />, { route: '/register' })
    const user = userEvent.setup()

    await user.type(screen.getByLabelText(/email/i), 'alice@example.com')
    await user.type(screen.getByLabelText(/username/i), 'alice')
    await user.type(screen.getByLabelText(/password/i), 'password123')
    await user.click(screen.getByRole('button', { name: /register/i }))

    await waitFor(() =>
      expect(api.register).toHaveBeenCalledWith('alice@example.com', 'alice', 'password123'),
    )
  })

  it('shows the server error message when the username is already taken', async () => {
    vi.mocked(api.register).mockRejectedValue(new Error('email or username already taken'))
    renderWithProviders(<Register />, { route: '/register' })
    const user = userEvent.setup()

    await user.type(screen.getByLabelText(/email/i), 'alice@example.com')
    await user.type(screen.getByLabelText(/username/i), 'alice')
    await user.type(screen.getByLabelText(/password/i), 'password123')
    await user.click(screen.getByRole('button', { name: /register/i }))

    expect(await screen.findByText('email or username already taken')).toBeInTheDocument()
  })

  async function fillAndSubmit(username: string, password: string) {
    renderWithProviders(<Register />, { route: '/register' })
    const user = userEvent.setup()
    await user.type(screen.getByLabelText(/email/i), 'alice@example.com')
    await user.type(screen.getByLabelText(/username/i), username)
    await user.type(screen.getByLabelText(/password/i), password)
    await user.click(screen.getByRole('button', { name: /register/i }))
  }

  it.each(['two words', 'a/b', '@alice', 'алиса'])(
    'does not submit a username the server would refuse (%s)',
    async (username) => {
      await fillAndSubmit(username, 'password123')

      expect(api.register).not.toHaveBeenCalled()
      expect(screen.getByLabelText(/username/i)).toBeInvalid()
    },
  )

  it('does not submit a password shorter than eight characters', async () => {
    await fillAndSubmit('alice', 'short12')

    expect(await screen.findByText('Password must be at least 8 characters.')).toBeInTheDocument()
    expect(api.register).not.toHaveBeenCalled()
  })

  it('accepts a long password, as a password manager would generate', async () => {
    vi.mocked(api.register).mockResolvedValue(undefined)
    const long = 'x'.repeat(64)

    await fillAndSubmit('alice', long)

    await waitFor(() => expect(api.register).toHaveBeenCalledWith('alice@example.com', 'alice', long))
  })

  it('links to the login page', () => {
    renderWithProviders(<Register />, { route: '/register' })
    expect(screen.getByRole('link', { name: /sign in/i })).toHaveAttribute('href', '/login')
  })
})
