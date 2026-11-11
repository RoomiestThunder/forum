import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as apiModule from '../lib/api'
import type { Notification } from '../lib/types'
import { useAuth } from '../stores/auth'
import { renderWithProviders, routeApi } from '../test/utils'
import NotificationBell from './NotificationBell'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return { ...actual, api: vi.fn() }
})

function notification(overrides: Partial<Notification>): Notification {
  return {
    id: 1,
    type: 'new_comment',
    payload: { post_id: 7, post_title: 'Hello', comment_id: 3, actor: 'bob' },
    read: false,
    created_at: new Date().toISOString(),
    ...overrides,
  } as Notification
}

async function openBellWith(notifications: Notification[]) {
  vi.mocked(apiModule.api).mockImplementation(
    routeApi({
      'GET /notifications/unread-count': { count: notifications.length },
      'GET /notifications': notifications,
    }) as typeof apiModule.api,
  )
  renderWithProviders(<NotificationBell />)
  await userEvent.setup().click(await screen.findByRole('button', { name: /notifications/i }))
}

beforeEach(() => {
  useAuth.getState().setSession({ userId: 1, username: 'me', accessToken: 't', refreshToken: 'r' })
})

afterEach(() => {
  vi.mocked(apiModule.api).mockReset()
  useAuth.getState().clearSession()
})

describe('NotificationBell', () => {
  it('describes each kind of notification', async () => {
    await openBellWith([
      notification({ id: 1, type: 'new_comment' }),
      notification({
        id: 2,
        type: 'new_reply',
        payload: { post_id: 7, post_title: 'Hello', comment_id: 4, parent_id: 3, actor: 'carol' },
      }),
      notification({ id: 3, type: 'new_mention', payload: { post_id: 7, post_title: 'Hello', comment_id: 5, actor: 'dave' } }),
      notification({ id: 4, type: 'new_like', payload: { post_id: 7, post_title: 'Hello', actor: 'erin' } }),
    ])

    expect(screen.getByText('bob commented on "Hello"')).toBeInTheDocument()
    expect(screen.getByText('carol replied to your comment on "Hello"')).toBeInTheDocument()
    expect(screen.getByText('dave mentioned you on "Hello"')).toBeInTheDocument()
    expect(screen.getByText('erin liked "Hello"')).toBeInTheDocument()
  })

  it('says so when there is nothing to show', async () => {
    await openBellWith([])
    expect(screen.getByText('Nothing here yet.')).toBeInTheDocument()
  })
})
