import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as apiModule from '../lib/api'
import { ApiError } from '../lib/api'
import { MAX_COMMENT_DEPTH, buildCommentTree } from '../lib/commentTree'
import type { Comment } from '../lib/types'
import { useAuth } from '../stores/auth'
import { renderWithProviders, routeApi } from '../test/utils'
import CommentThread from './CommentThread'

vi.mock('../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../lib/api')>('../lib/api')
  return { ...actual, api: vi.fn() }
})

function comment(id: number, overrides: Partial<Comment> = {}): Comment {
  return {
    id,
    post_id: 10,
    user_id: 100 + id,
    parent_id: null,
    depth: 0,
    content: `text of ${id}`,
    created_at: new Date().toISOString(),
    author: `user${id}`,
    likes: 0,
    dislikes: 0,
    deleted: false,
    ...overrides,
  }
}

function renderThread(comments: Comment[]) {
  const [node] = buildCommentTree(comments)
  return renderWithProviders(<CommentThread node={node} postId={10} />)
}

/** The Reply button inside the open reply form, as opposed to the one under the comment. */
const replyFormSubmit = () => {
  const form = screen.getByPlaceholderText('Reply to user1…').closest('form')!
  return within(form).getByRole('button', { name: 'Reply' })
}

const signIn = () =>
  useAuth.getState().setSession({ userId: 1, username: 'me', accessToken: 't', refreshToken: 'r' })

beforeEach(() => {
  // CommentItem asks who the viewer is (for staff controls).
  vi.mocked(apiModule.api).mockImplementation(
    routeApi({
      'GET /me': { id: 1, username: 'me', is_admin: false, is_moderator: false },
    }) as typeof apiModule.api,
  )
})

afterEach(() => {
  vi.mocked(apiModule.api).mockReset()
  useAuth.getState().clearSession()
})

describe('CommentThread', () => {
  it('renders a comment and its replies', () => {
    renderThread([comment(1), comment(2, { parent_id: 1, depth: 1 }), comment(3, { parent_id: 2, depth: 2 })])

    expect(screen.getByText('text of 1')).toBeInTheDocument()
    expect(screen.getByText('text of 2')).toBeInTheDocument()
    expect(screen.getByText('text of 3')).toBeInTheDocument()
  })

  it('renders a deleted comment as a placeholder while keeping its replies', () => {
    renderThread([
      comment(1, { deleted: true, content: '', author: '', user_id: 0 }),
      comment(2, { parent_id: 1, depth: 1 }),
    ])

    expect(screen.getByText('[deleted]')).toBeInTheDocument()
    expect(screen.getByText('text of 2')).toBeInTheDocument()
  })

  it('shows no Reply button to a signed-out visitor', () => {
    renderThread([comment(1)])
    expect(screen.queryByRole('button', { name: 'Reply' })).not.toBeInTheDocument()
  })

  it('shows no Reply button on a deleted comment', () => {
    signIn()
    renderThread([comment(1, { deleted: true, content: '' })])
    expect(screen.queryByRole('button', { name: 'Reply' })).not.toBeInTheDocument()
  })

  it('stops offering Reply at the maximum depth', () => {
    signIn()
    renderThread([comment(1, { depth: MAX_COMMENT_DEPTH })])
    expect(screen.queryByRole('button', { name: 'Reply' })).not.toBeInTheDocument()
  })

  it('still offers Reply one level short of the maximum depth', () => {
    signIn()
    renderThread([comment(1, { depth: MAX_COMMENT_DEPTH - 1 })])
    expect(screen.getByRole('button', { name: 'Reply' })).toBeInTheDocument()
  })

  it('posts a reply with the comment as its parent and closes the form', async () => {
    signIn()
    const create = vi.fn().mockReturnValue({ id: 5 })
    vi.mocked(apiModule.api).mockImplementation(
      routeApi({
        'GET /me': { id: 1, username: 'me', is_admin: false, is_moderator: false },
        'POST /posts/10': create,
      }) as typeof apiModule.api,
    )
    renderThread([comment(1)])
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: 'Reply' }))
    await user.type(screen.getByPlaceholderText('Reply to user1…'), 'a thoughtful answer')
    await user.click(replyFormSubmit())

    await waitFor(() => expect(create).toHaveBeenCalledTimes(1))
    const init = create.mock.calls[0][0] as RequestInit
    expect(JSON.parse(init.body as string)).toEqual({ content: 'a thoughtful answer', parent_id: 1 })
    await waitFor(() => expect(screen.queryByPlaceholderText('Reply to user1…')).not.toBeInTheDocument())
  })

  it('shows why a reply was refused and keeps the form open', async () => {
    signIn()
    vi.mocked(apiModule.api).mockImplementation(
      routeApi({
        'GET /me': { id: 1, username: 'me', is_admin: false, is_moderator: false },
        'POST /posts/10': () => {
          throw new ApiError(400, 'you can\'t reply to a deleted comment')
        },
      }) as typeof apiModule.api,
    )
    renderThread([comment(1)])
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: 'Reply' }))
    await user.type(screen.getByPlaceholderText('Reply to user1…'), 'too late')
    await user.click(replyFormSubmit())

    expect(await screen.findByRole('alert')).toHaveTextContent("you can't reply to a deleted comment")
    expect(screen.getByPlaceholderText('Reply to user1…')).toBeInTheDocument()
  })

  it('closes the reply form on Cancel without posting anything', async () => {
    signIn()
    renderThread([comment(1)])
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: 'Reply' }))
    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(screen.queryByPlaceholderText('Reply to user1…')).not.toBeInTheDocument()
    expect(vi.mocked(apiModule.api)).not.toHaveBeenCalledWith('/posts/10', expect.anything())
  })
})
