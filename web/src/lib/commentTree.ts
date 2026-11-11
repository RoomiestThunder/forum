import type { Comment } from './types'

/** How many levels of replies may sit below a top-level comment (mirrors the API). */
export const MAX_COMMENT_DEPTH = 5

export interface CommentNode {
  comment: Comment
  replies: CommentNode[]
}

/**
 * Turns the API's flat, oldest-first comment list into a tree. Siblings keep
 * the order they arrived in. A comment whose parent isn't in the list (which
 * the server shouldn't produce, but a stale or partial response could) is
 * shown at the top level rather than silently dropped.
 */
export function buildCommentTree(comments: Comment[]): CommentNode[] {
  const nodes = new Map<number, CommentNode>()
  for (const comment of comments) {
    nodes.set(comment.id, { comment, replies: [] })
  }

  const roots: CommentNode[] = []
  for (const comment of comments) {
    const node = nodes.get(comment.id)!
    const parent = comment.parent_id === null ? undefined : nodes.get(comment.parent_id)
    if (parent) parent.replies.push(node)
    else roots.push(node)
  }
  return roots
}

/** Number of comments that are still live (tombstones don't count as comments). */
export function countLive(comments: Comment[]): number {
  return comments.filter((c) => !c.deleted).length
}
