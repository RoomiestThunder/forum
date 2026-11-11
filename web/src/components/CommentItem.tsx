import { useState } from 'react'
import { CornerDownRight, Pencil, Trash2 } from 'lucide-react'
import { Link } from 'react-router'
import {
  useAdminDeleteComment,
  useDeleteComment,
  useMe,
  useUpdateComment,
  useVoteComment,
} from '../lib/queries'
import { timeAgo } from '../lib/time'
import type { Comment } from '../lib/types'
import { useAuth } from '../stores/auth'
import MentionText from './MentionText'
import MentionTextarea from './MentionTextarea'
import ReportButton from './ReportButton'
import VoteButtons from './VoteButtons'

interface Props {
  comment: Comment
  /** Shown as a Reply button when given; the parent decides whether replying is allowed. */
  onReply?: () => void
}

export default function CommentItem({ comment, onReply }: Props) {
  const userId = useAuth((s) => s.userId)
  const loggedIn = useAuth((s) => s.refreshToken !== null)
  const isOwn = userId === comment.user_id
  const { data: me } = useMe()
  const isStaff = !!me && (me.is_admin || me.is_moderator)

  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(comment.content)

  const vote = useVoteComment(comment.post_id)
  const update = useUpdateComment(comment.post_id)
  const remove = useDeleteComment(comment.post_id)
  const adminRemove = useAdminDeleteComment(comment.post_id)

  const saveEdit = () => {
    const content = draft.trim()
    if (!content) return
    update.mutate(
      { commentId: comment.id, content },
      { onSuccess: () => setEditing(false) },
    )
  }

  // A deleted comment that still has replies under it: a placeholder, with no
  // author, text or actions.
  if (comment.deleted) {
    return (
      <div className="rounded-lg border border-dashed border-zinc-200 px-3 py-2 text-sm italic text-zinc-400 dark:border-zinc-800 dark:text-zinc-500">
        [deleted]
      </div>
    )
  }

  return (
    <div className="rounded-lg border border-zinc-200 bg-white p-3 dark:border-zinc-800 dark:bg-zinc-900">
      <div className="flex items-center gap-2 text-sm text-zinc-500 dark:text-zinc-400">
        <Link to={`/u/${comment.author}`} className="font-medium text-zinc-700 hover:underline dark:text-zinc-300">
          {comment.author}
        </Link>
        <span aria-hidden>·</span>
        <time dateTime={comment.created_at}>{timeAgo(comment.created_at)}</time>
        {!editing && (isOwn || isStaff) && (
          <span className="ml-auto flex items-center gap-1">
            {isOwn && (
              <button
                type="button"
                onClick={() => {
                  setDraft(comment.content)
                  setEditing(true)
                }}
                aria-label="Edit comment"
                className="rounded p-1 text-zinc-400 transition hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
              >
                <Pencil size={14} />
              </button>
            )}
            <button
              type="button"
              onClick={() => {
                if (window.confirm('Delete this comment?')) {
                  if (isOwn) remove.mutate(comment.id)
                  else adminRemove.mutate(comment.id)
                }
              }}
              disabled={remove.isPending || adminRemove.isPending}
              aria-label="Delete comment"
              className="rounded p-1 text-zinc-400 transition hover:bg-zinc-100 hover:text-red-600 disabled:opacity-50 dark:hover:bg-zinc-800 dark:hover:text-red-400"
            >
              <Trash2 size={14} />
            </button>
          </span>
        )}
        {!editing && !isOwn && loggedIn && (
          <ReportButton targetType="comment" targetId={comment.id} className={isStaff ? '' : 'ml-auto'} />
        )}
      </div>

      {editing ? (
        <div className="mt-2">
          <MentionTextarea
            value={draft}
            onChange={setDraft}
            maxLength={500}
            rows={3}
            className="w-full rounded-lg border border-zinc-300 bg-white px-3 py-2 text-sm outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-zinc-700 dark:bg-zinc-950"
          />
          <div className="mt-1.5 flex gap-2">
            <button
              type="button"
              onClick={saveEdit}
              disabled={update.isPending || !draft.trim()}
              className="rounded-lg bg-indigo-600 px-3 py-1 text-sm font-medium text-white transition hover:bg-indigo-500 disabled:opacity-50"
            >
              Save
            </button>
            <button
              type="button"
              onClick={() => setEditing(false)}
              className="rounded-lg px-3 py-1 text-sm font-medium text-zinc-600 transition hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800"
            >
              Cancel
            </button>
          </div>
        </div>
      ) : (
        <p className="mt-1.5 whitespace-pre-wrap text-sm">
          <MentionText text={comment.content} />
        </p>
      )}

      {!editing && (
        <div className="mt-2 flex items-center gap-3">
          <VoteButtons
            likes={comment.likes}
            dislikes={comment.dislikes}
            onVote={(isLike) => vote.mutate({ commentId: comment.id, isLike })}
            disabled={vote.isPending}
          />
          {onReply && (
            <button
              type="button"
              onClick={onReply}
              className="flex items-center gap-1 rounded-full px-2 py-1 text-xs font-medium text-zinc-500 transition hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
            >
              <CornerDownRight size={13} />
              Reply
            </button>
          )}
        </div>
      )}
    </div>
  )
}
