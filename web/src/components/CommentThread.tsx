import { useState } from 'react'
import { MAX_COMMENT_DEPTH, type CommentNode } from '../lib/commentTree'
import { useAuth } from '../stores/auth'
import CommentForm from './CommentForm'
import CommentItem from './CommentItem'

/** One comment with its reply box and, recursively, everything beneath it. */
export default function CommentThread({ node, postId }: { node: CommentNode; postId: number }) {
  const loggedIn = useAuth((s) => s.refreshToken !== null)
  const [replying, setReplying] = useState(false)
  const { comment, replies } = node

  // The server enforces the same rules; hiding the button just avoids offering
  // an action that would be refused.
  const canReply = loggedIn && !comment.deleted && comment.depth < MAX_COMMENT_DEPTH

  return (
    <div>
      <CommentItem comment={comment} onReply={canReply ? () => setReplying(true) : undefined} />

      {replying && (
        <div className="mt-2 ml-3 pl-3">
          <CommentForm
            postId={postId}
            parentId={comment.id}
            placeholder={`Reply to ${comment.author}…`}
            submitLabel="Reply"
            autoFocus
            onDone={() => setReplying(false)}
            onCancel={() => setReplying(false)}
          />
        </div>
      )}

      {replies.length > 0 && (
        <div className="mt-2 ml-3 space-y-2 border-l-2 border-zinc-200 pl-3 dark:border-zinc-800">
          {replies.map((reply) => (
            <CommentThread key={reply.comment.id} node={reply} postId={postId} />
          ))}
        </div>
      )}
    </div>
  )
}
