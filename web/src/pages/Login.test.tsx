import { beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '../test/utils'
import * as api from '../lib/api'
import Login from './Login'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return { ...actual, login: vi.fn() }
})

describe('Login', () => {
  beforeEach(() => {
    vi.mocked(api.login).mockReset()
  })

  it('submits the entered email/username and password to login()', async () => {
    vi.mocked(api.login).mockResolvedValue(undefined)
    renderWithProviders(<Login />, { route: '/login' })
    const user = userEvent.setup()

    await user.type(screen.getByLabelText(/email or username/i), 'alice')
    await user.type(screen.getByLabelText(/password/i), 'password123')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() => expect(api.login).toHaveBeenCalledWith('alice', 'password123'))
  })

  it('shows the server error message when login fails', async () => {
    vi.mocked(api.login).mockRejectedValue(new Error('invalid credentials'))
    renderWithProviders(<Login />, { route: '/login' })
    const user = userEvent.setup()

    await user.type(screen.getByLabelText(/email or username/i), 'alice')
    await user.type(screen.getByLabelText(/password/i), 'wrong-password')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    expect(await screen.findByText('invalid credentials')).toBeInTheDocument()
  })

  it('links to the forgot-password and register pages', () => {
    renderWithProviders(<Login />, { route: '/login' })
    expect(screen.getByRole('link', { name: /forgot password/i })).toHaveAttribute('href', '/forgot-password')
    expect(screen.getByRole('link', { name: /register/i })).toHaveAttribute('href', '/register')
  })
})
