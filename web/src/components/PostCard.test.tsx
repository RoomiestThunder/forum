import { describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'
import type { Post } from '../lib/types'
import { renderWithProviders } from '../test/utils'
import PostCard from './PostCard'

const basePost: Post = {
  id: 1,
  user_id: 5,
  title: 'Hello world',
  content: 'Some content here',
  created_at: new Date().toISOString(),
  author: 'alice',
  likes: 3,
  dislikes: 1,
  categories: [{ id: 1, name: 'General' }],
  tags: [{ id: 1, name: 'intro' }],
  comment_count: 2,
  pinned: false,
  draft: false,
}

describe('PostCard', () => {
  it('renders title, author, category, tag, and vote/comment counts', () => {
    renderWithProviders(<PostCard post={basePost} />)
    expect(screen.getByRole('link', { name: 'Hello world' })).toHaveAttribute('href', '/post/1')
    expect(screen.getByRole('link', { name: 'alice' })).toHaveAttribute('href', '/u/alice')
    expect(screen.getByText('General')).toBeInTheDocument()
    expect(screen.getByText('#intro')).toBeInTheDocument()
    expect(screen.getByLabelText('Like')).toHaveTextContent('3')
    expect(screen.getByLabelText('Dislike')).toHaveTextContent('1')
  })

  it('shows a Draft badge for draft posts', () => {
    renderWithProviders(<PostCard post={{ ...basePost, draft: true }} />)
    expect(screen.getByText('Draft')).toBeInTheDocument()
  })

  it('does not show a Draft badge for published posts', () => {
    renderWithProviders(<PostCard post={basePost} />)
    expect(screen.queryByText('Draft')).not.toBeInTheDocument()
  })

  it('omits the tag/category rows when there are none', () => {
    renderWithProviders(<PostCard post={{ ...basePost, categories: [], tags: [] }} />)
    expect(screen.queryByText('General')).not.toBeInTheDocument()
    expect(screen.queryByText('#intro')).not.toBeInTheDocument()
  })
})
