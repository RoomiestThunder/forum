import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as apiModule from '../lib/api'
import type { AuditEntry } from '../lib/types'
import { useAuth } from '../stores/auth'
import { renderWithProviders, routeApi } from '../test/utils'
import Admin from './Admin'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return { ...actual, api: vi.fn() }
})

const admin = { id: 1, username: 'admin', is_admin: true, is_moderator: false }
const moderator = { id: 2, username: 'mod', is_admin: false, is_moderator: true }
const member = { id: 3, username: 'bob', is_admin: false, is_moderator: false }

function auditEntry(id: number, overrides: Partial<AuditEntry> = {}): AuditEntry {
  return {
    id,
    actor_id: 1,
    actor: 'admin',
    action: 'post.delete',
    target_type: 'post',
    target_id: id,
    details: { title: `Post ${id}`, author: 'mallory', excerpt: `body of ${id}` },
    created_at: new Date().toISOString(),
    ...overrides,
  }
}

function serve(me: object, extra: Record<string, unknown> = {}) {
  vi.mocked(apiModule.api).mockImplementation(
    routeApi({
      'GET /me': me,
      'GET /admin/reports?status=open': [],
      'GET /admin/users?page=1': { users: [], total: 0, page: 1 },
      ...extra,
    }) as typeof apiModule.api,
  )
}

beforeEach(() => {
  useAuth.getState().setSession({ userId: 1, username: 'admin', accessToken: 't', refreshToken: 'r' })
})

afterEach(() => {
  vi.mocked(apiModule.api).mockReset()
  useAuth.getState().clearSession()
})

describe('Admin', () => {
  it('gives admins the Reports, Users and Audit log tabs', async () => {
    serve(admin)
    renderWithProviders(<Admin />)

    expect(await screen.findByRole('button', { name: 'Reports' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Users' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Audit log' })).toBeInTheDocument()
  })

  it('gives moderators only the Reports tab', async () => {
    serve(moderator)
    renderWithProviders(<Admin />)

    expect(await screen.findByRole('button', { name: 'Reports' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Users' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Audit log' })).not.toBeInTheDocument()
  })

  it('shows nothing of the admin panel to an ordinary member', async () => {
    serve(member)
    const { container } = renderWithProviders(<Admin />)

    // Wait for /me to resolve (the loading spinner goes away); only then is the
    // absence of the panel meaningful rather than just "not rendered yet".
    await waitFor(() => expect(container.querySelector('.animate-spin')).toBeNull())
    expect(screen.queryByRole('heading', { name: 'Admin' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Reports' })).not.toBeInTheDocument()
  })

  it('lists audit entries with what was removed', async () => {
    serve(admin, {
      'GET /admin/audit?page=1': {
        entries: [
          auditEntry(2, { action: 'user.ban', target_type: 'user', details: { username: 'mallory' }, actor: 'mod' }),
          auditEntry(1),
        ],
        total: 2,
        page: 1,
      },
    })
    renderWithProviders(<Admin />)
    await userEvent.setup().click(await screen.findByRole('button', { name: 'Audit log' }))

    expect(await screen.findByText(/banned mallory/)).toBeInTheDocument()
    expect(screen.getByText(/deleted "Post 1" by mallory/)).toBeInTheDocument()
    expect(screen.getByText('body of 1')).toBeInTheDocument() // the kept excerpt
    expect(screen.getByRole('link', { name: 'mod' })).toHaveAttribute('href', '/u/mod')
  })

  it('says so when nothing has been recorded', async () => {
    serve(admin, { 'GET /admin/audit?page=1': { entries: [], total: 0, page: 1 } })
    renderWithProviders(<Admin />)
    await userEvent.setup().click(await screen.findByRole('button', { name: 'Audit log' }))

    expect(await screen.findByText('No moderation actions recorded yet.')).toBeInTheDocument()
  })

  it('pages through the audit log', async () => {
    const page = (n: number) => ({ entries: [auditEntry(n)], total: 120, page: n })
    serve(admin, {
      'GET /admin/audit?page=1': page(1),
      'GET /admin/audit?page=2': page(2),
    })
    renderWithProviders(<Admin />)
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Audit log' }))

    expect(await screen.findByText('Page 1 of 3')).toBeInTheDocument() // 120 entries, 50 per page
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()

    await user.click(screen.getByRole('button', { name: 'Next' }))

    expect(await screen.findByText('Page 2 of 3')).toBeInTheDocument()
    expect(await screen.findByText(/deleted "Post 2"/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Previous' })).toBeEnabled()
  })

  it('does not show a pager when everything fits on one page', async () => {
    serve(admin, { 'GET /admin/audit?page=1': { entries: [auditEntry(1)], total: 1, page: 1 } })
    renderWithProviders(<Admin />)
    await userEvent.setup().click(await screen.findByRole('button', { name: 'Audit log' }))

    await screen.findByText(/deleted "Post 1"/)
    expect(screen.queryByRole('navigation', { name: 'Pagination' })).not.toBeInTheDocument()
  })
})
