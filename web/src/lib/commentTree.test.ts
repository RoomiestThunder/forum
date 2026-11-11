import { describe, expect, it } from 'vitest'
import { buildCommentTree, countLive } from './commentTree'
import type { Comment } from './types'

let nextId = 1
function comment(overrides: Partial<Comment> = {}): Comment {
  const id = nextId++
  return {
    id,
    post_id: 1,
    user_id: 1,
    parent_id: null,
    depth: 0,
    content: `comment ${id}`,
    created_at: '2026-01-01T00:00:00Z',
    author: 'alice',
    likes: 0,
    dislikes: 0,
    deleted: false,
    ...overrides,
  }
}

const ids = (nodes: { comment: Comment }[]) => nodes.map((n) => n.comment.id)

describe('buildCommentTree', () => {
  it('returns no roots for no comments', () => {
    expect(buildCommentTree([])).toEqual([])
  })

  it('keeps top-level comments in the order given', () => {
    const [a, b, c] = [comment(), comment(), comment()]
    expect(ids(buildCommentTree([a, b, c]))).toEqual([a.id, b.id, c.id])
  })

  it('nests replies under their parent, at any depth', () => {
    const root = comment()
    const reply = comment({ parent_id: root.id, depth: 1 })
    const nested = comment({ parent_id: reply.id, depth: 2 })

    const [tree] = buildCommentTree([root, reply, nested])

    expect(tree.comment.id).toBe(root.id)
    expect(ids(tree.replies)).toEqual([reply.id])
    expect(ids(tree.replies[0].replies)).toEqual([nested.id])
  })

  it('keeps sibling replies in the order given', () => {
    const root = comment()
    const first = comment({ parent_id: root.id, depth: 1 })
    const second = comment({ parent_id: root.id, depth: 1 })

    const [tree] = buildCommentTree([root, first, second])

    expect(ids(tree.replies)).toEqual([first.id, second.id])
  })

  it('separates independent threads', () => {
    const a = comment()
    const b = comment()
    const replyToB = comment({ parent_id: b.id, depth: 1 })

    const roots = buildCommentTree([a, b, replyToB])

    expect(ids(roots)).toEqual([a.id, b.id])
    expect(roots[0].replies).toEqual([])
    expect(ids(roots[1].replies)).toEqual([replyToB.id])
  })

  it('shows a reply at the top level instead of dropping it when its parent is missing', () => {
    const orphan = comment({ parent_id: 9999, depth: 1 })
    expect(ids(buildCommentTree([orphan]))).toEqual([orphan.id])
  })

  it('does not require a parent to precede its replies in the list', () => {
    const root = comment()
    const reply = comment({ parent_id: root.id, depth: 1 })

    const [tree] = buildCommentTree([reply, root])

    expect(tree.comment.id).toBe(root.id)
    expect(ids(tree.replies)).toEqual([reply.id])
  })
})

describe('countLive', () => {
  it('does not count tombstones as comments', () => {
    expect(countLive([comment(), comment({ deleted: true }), comment()])).toBe(2)
  })
})
