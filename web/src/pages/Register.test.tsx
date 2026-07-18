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

  it('links to the login page', () => {
    renderWithProviders(<Register />, { route: '/register' })
    expect(screen.getByRole('link', { name: /sign in/i })).toHaveAttribute('href', '/login')
  })
})
