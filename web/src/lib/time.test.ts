import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fullDate, timeAgo } from './time'

const NOW = new Date('2026-06-15T12:00:00Z')
const ago = (seconds: number) => new Date(NOW.getTime() - seconds * 1000).toISOString()

describe('timeAgo', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(NOW)
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it.each([
    [0, 'just now'],
    [30, 'just now'],
    [59, 'just now'],
    [60, '1 minute ago'],
    [5 * 60, '5 minutes ago'],
    [3600, '1 hour ago'],
    [3 * 3600, '3 hours ago'],
    [24 * 3600, 'yesterday'],
    [3 * 24 * 3600, '3 days ago'],
    [30 * 24 * 3600, 'last month'],
    [365 * 24 * 3600, 'last year'],
    [3 * 365 * 24 * 3600, '3 years ago'],
  ])('%i seconds ago reads %s', (seconds, expected) => {
    expect(timeAgo(ago(seconds))).toBe(expected)
  })

  it('describes a time in the future', () => {
    expect(timeAgo(ago(-2 * 3600))).toBe('in 2 hours')
  })
})

describe('fullDate', () => {
  it('formats an ISO timestamp as a readable date and time', () => {
    expect(fullDate('2026-06-15T12:34:00Z')).toMatch(/15 Jun 2026/)
  })
})
