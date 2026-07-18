import { describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '../test/utils'
import MentionText from './MentionText'

describe('MentionText', () => {
  it('renders plain text with no mentions unchanged', () => {
    renderWithProviders(<MentionText text="just a normal comment" />)
    expect(screen.getByText('just a normal comment')).toBeInTheDocument()
  })

  it('turns @username into a profile link', () => {
    renderWithProviders(<MentionText text="hey @alice, check this out" />)
    const link = screen.getByRole('link', { name: '@alice' })
    expect(link).toHaveAttribute('href', '/u/alice')
  })

  it('handles multiple distinct mentions in one string', () => {
    renderWithProviders(<MentionText text="@alice and @bob_2 were here" />)
    expect(screen.getByRole('link', { name: '@alice' })).toHaveAttribute('href', '/u/alice')
    expect(screen.getByRole('link', { name: '@bob_2' })).toHaveAttribute('href', '/u/bob_2')
  })

  it('does not link an @ token shorter than the minimum username length', () => {
    renderWithProviders(<MentionText text="contact us @ab for help" />)
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
