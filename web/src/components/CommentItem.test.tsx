import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Comment } from '../lib/types'
import { useAuth } from '../stores/auth'
import { renderWithProviders } from '../test/utils'
import CommentItem from './CommentItem'

const baseComment: Comment = {
  id: 1,
  post_id: 10,
  user_id: 1,
  content: 'hey @alice, nice work on this',
  created_at: new Date().toISOString(),
  author: 'bob',
  likes: 2,
  dislikes: 0,
}

describe('CommentItem', () => {
  afterEach(() => {
    useAuth.getState().clearSession()
  })

  it('renders the author and content with a mention turned into a profile link', () => {
    renderWithProviders(<CommentItem comment={baseComment} />)
    expect(screen.getByText('bob')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '@alice' })).toHaveAttribute('href', '/u/alice')
  })

  it("hides edit/delete controls for someone else's comment", () => {
    useAuth.getState().setSession({ userId: 99, username: 'someone', accessToken: 't', refreshToken: 'r' })
    renderWithProviders(<CommentItem comment={baseComment} />)
    expect(screen.queryByLabelText('Edit comment')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Delete comment')).not.toBeInTheDocument()
  })

  it('shows edit/delete controls for your own comment', () => {
    useAuth.getState().setSession({ userId: 1, username: 'bob', accessToken: 't', refreshToken: 'r' })
    renderWithProviders(<CommentItem comment={baseComment} />)
    expect(screen.getByLabelText('Edit comment')).toBeInTheDocument()
    expect(screen.getByLabelText('Delete comment')).toBeInTheDocument()
  })

  describe('editing', () => {
    beforeEach(() => {
      useAuth.getState().setSession({ userId: 1, username: 'bob', accessToken: 't', refreshToken: 'r' })
    })

    it('opens a textarea prefilled with the content, and Cancel reverts it', async () => {
      renderWithProviders(<CommentItem comment={baseComment} />)
      const user = userEvent.setup()

      await user.click(screen.getByLabelText('Edit comment'))
      expect(screen.getByRole('textbox')).toHaveValue(baseComment.content)

      await user.click(screen.getByRole('button', { name: /cancel/i }))
      expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
      expect(screen.getByText(/nice work on this/)).toBeInTheDocument()
    })

    it('disables Save while the draft is empty', async () => {
      renderWithProviders(<CommentItem comment={baseComment} />)
      const user = userEvent.setup()

      await user.click(screen.getByLabelText('Edit comment'))
      const textarea = screen.getByRole('textbox')
      await user.clear(textarea)

      expect(screen.getByRole('button', { name: /^save$/i })).toBeDisabled()
    })
  })
})
