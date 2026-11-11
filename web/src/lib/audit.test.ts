import { describe, expect, it } from 'vitest'
import { auditExcerpt, describeAudit } from './audit'
import type { AuditEntry } from './types'

function entry(action: string, details: Record<string, unknown> = {}, overrides: Partial<AuditEntry> = {}): AuditEntry {
  return {
    id: 1,
    actor_id: 1,
    actor: 'admin',
    action,
    target_type: 'post',
    target_id: 7,
    details,
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('describeAudit', () => {
  it.each([
    ['post.pin', { title: 'Hello' }, 'pinned "Hello"'],
    ['post.unpin', { title: 'Hello' }, 'unpinned "Hello"'],
    ['post.delete', { title: 'Hello', author: 'mallory' }, 'deleted "Hello" by mallory'],
    ['comment.delete', { author: 'mallory' }, 'deleted a comment by mallory'],
    ['user.ban', { username: 'mallory' }, 'banned mallory'],
    ['user.unban', { username: 'mallory' }, 'unbanned mallory'],
  ])('%s', (action, details, want) => {
    expect(describeAudit(entry(action, details))).toBe(want)
  })

  it('describes a role change with the old and new role', () => {
    const text = describeAudit(
      entry('user.role', { username: 'bob', was_admin: false, was_moderator: false, is_admin: false, is_moderator: true }),
    )
    expect(text).toBe('changed bob from Member to Moderator')
  })

  it('treats admin as outranking moderator', () => {
    const text = describeAudit(
      entry('user.role', { username: 'bob', was_admin: true, was_moderator: true, is_admin: false, is_moderator: false }),
    )
    expect(text).toBe('changed bob from Admin to Member')
  })

  it('names a report by its id', () => {
    expect(describeAudit(entry('report.resolve', {}, { target_type: 'report', target_id: 12 }))).toBe('resolved report #12')
  })

  it('falls back gracefully when the snapshot is missing details', () => {
    expect(describeAudit(entry('post.delete', {}))).toBe('deleted a post by unknown')
    expect(describeAudit(entry('user.ban', {}, { target_type: 'user', target_id: 4 }))).toBe('banned user #4')
  })

  it('still shows an action this client does not know', () => {
    expect(describeAudit(entry('thread.lock', {}, { target_type: 'post', target_id: 9 }))).toBe('thread.lock on post #9')
  })
})

describe('auditExcerpt', () => {
  it('returns the kept text', () => {
    expect(auditExcerpt(entry('post.delete', { excerpt: 'the removed words' }))).toBe('the removed words')
  })

  it('returns null when there is none', () => {
    expect(auditExcerpt(entry('post.delete', {}))).toBeNull()
    expect(auditExcerpt(entry('post.delete', { excerpt: '' }))).toBeNull()
    expect(auditExcerpt(entry('post.delete', { excerpt: 42 }))).toBeNull()
  })
})
