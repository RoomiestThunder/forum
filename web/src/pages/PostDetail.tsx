import { useState } from 'react'
import { Bookmark, Loader2, Pencil, Trash2 } from 'lucide-react'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Link, useNavigate, useParams } from 'react-router'
import CommentItem from '../components/CommentItem'
import VoteButtons from '../components/VoteButtons'
import {
  useCreateComment,
  useDeletePost,
  usePost,
  useToggleBookmark,
  useVotePost,
} from '../lib/queries'
import { fullDate, timeAgo } from '../lib/time'
import { useAuth } from '../stores/auth'

export default function PostDetail() {
  const { id } = useParams()
  const postId = Number(id)
  const navigate = useNavigate()

  const userId = useAuth((s) => s.userId)
  const loggedIn = useAuth((s) => s.refreshToken !== null)

  const { data, isLoading, isError, error } = usePost(postId)
  const vote = useVotePost()
  const deletePost = useDeletePost()
  const createComment = useCreateComment(postId)
  const toggleBookmark = useToggleBookmark(postId)

  const [commentDraft, setCommentDraft] = useState('')

  if (isLoading) {
    return (
      <div className="flex justify-center py-16">
        <Loader2 className="animate-spin text-zinc-400" />
      </div>
    )
  }

  if (isError || !data) {
    return (
      <p className="rounded-lg bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/50 dark:text-red-300">
        {error?.message ?? 'Post not found'}
      </p>
    )
  }

  const { post } = data
  const comments = data.comments ?? []
  const isOwn = userId === post.user_id

  const submitComment = () => {
    const content = commentDraft.trim()
    if (!content) return
    createComment.mutate(content, { onSuccess: () => setCommentDraft('') })
  }

  return (
    <div className="space-y-6">
      <article>
        <div className="flex items-center gap-2 text-sm text-zinc-500 dark:text-zinc-400">
          <Link to={`/u/${post.author}`} className="font-medium text-zinc-700 hover:underline dark:text-zinc-300">
            {post.author}
          </Link>
          <span aria-hidden>·</span>
          <time dateTime={post.created_at} title={fullDate(post.created_at)}>
            {timeAgo(post.created_at)}
          </time>
          {post.categories?.map((c) => (
            <Link
              key={c.id}
              to={`/?category=${c.id}`}
              className="rounded-full bg-zinc-100 px-2 py-0.5 text-xs font-medium text-zinc-600 transition hover:bg-zinc-200 dark:bg-zinc-800 dark:text-zinc-300 dark:hover:bg-zinc-700"
            >
              {c.name}
            </Link>
          ))}
          {isOwn && (
            <span className="ml-auto flex items-center gap-1">
              <Link
                to={`/post/${post.id}/edit`}
                aria-label="Edit post"
                className="rounded p-1.5 text-zinc-400 transition hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
              >
                <Pencil size={16} />
              </Link>
              <button
                type="button"
                onClick={() => {
                  if (window.confirm('Delete this post?')) {
                    deletePost.mutate(post.id, { onSuccess: () => navigate('/') })
                  }
                }}
                disabled={deletePost.isPending}
                aria-label="Delete post"
                className="rounded p-1.5 text-zinc-400 transition hover:bg-zinc-100 hover:text-red-600 disabled:opacity-50 dark:hover:bg-zinc-800 dark:hover:text-red-400"
              >
                <Trash2 size={16} />
              </button>
            </span>
          )}
        </div>

        <h1 className="mt-2 text-2xl font-bold">{post.title}</h1>

        {post.image_url && (
          <img
            src={post.image_url}
            alt=""
            className="mt-4 max-h-96 w-full rounded-lg border border-zinc-200 object-cover dark:border-zinc-800"
          />
        )}

        <div className="prose prose-zinc mt-4 max-w-none dark:prose-invert">
          <Markdown remarkPlugins={[remarkGfm]}>{post.content}</Markdown>
        </div>

        {post.tags && post.tags.length > 0 && (
          <div className="mt-3 flex flex-wrap gap-2">
            {post.tags.map((t) => (
              <Link
                key={t.id}
                to={`/?tag=${t.name}`}
                className="text-sm font-medium text-indigo-600 hover:underline dark:text-indigo-400"
              >
                #{t.name}
              </Link>
            ))}
          </div>
        )}

        <div className="mt-4 flex items-center gap-3">
          <VoteButtons
            likes={post.likes}
            dislikes={post.dislikes}
            onVote={(isLike) => vote.mutate({ postId: post.id, isLike })}
            disabled={vote.isPending}
          />
          {loggedIn && (
            <button
              type="button"
              onClick={() => toggleBookmark.mutate(!data.is_bookmarked)}
              disabled={toggleBookmark.isPending}
              aria-pressed={data.is_bookmarked}
              className={`flex items-center gap-1.5 rounded-full px-2.5 py-1 text-sm transition disabled:opacity-50 ${
                data.is_bookmarked
                  ? 'text-indigo-600 dark:text-indigo-400'
                  : 'text-zinc-500 hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100'
              }`}
            >
              <Bookmark size={15} fill={data.is_bookmarked ? 'currentColor' : 'none'} />
              {data.is_bookmarked ? 'Saved' : 'Save'}
            </button>
          )}
        </div>
      </article>

      <section>
        <h2 className="mb-3 text-lg font-semibold">
          Comments <span className="text-zinc-400">({comments.length})</span>
        </h2>

        <div className="space-y-2">
          {comments.map((comment) => (
            <CommentItem key={comment.id} comment={comment} />
          ))}
          {comments.length === 0 && (
            <p className="py-4 text-sm text-zinc-500 dark:text-zinc-400">No comments yet.</p>
          )}
        </div>

        {loggedIn ? (
          <form
            className="mt-4"
            onSubmit={(e) => {
              e.preventDefault()
              submitComment()
            }}
          >
            {createComment.isError && (
              <p className="mb-2 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950/50 dark:text-red-300">
                {createComment.error.message}
              </p>
            )}
            <textarea
              value={commentDraft}
              onChange={(e) => setCommentDraft(e.target.value)}
              placeholder="Write a comment…"
              maxLength={500}
              rows={3}
              className="w-full rounded-lg border border-zinc-300 bg-white px-3 py-2 text-sm outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-zinc-700 dark:bg-zinc-900"
            />
            <div className="mt-2 flex items-center justify-between">
              <span className="text-xs text-zinc-400">{commentDraft.length}/500</span>
              <button
                type="submit"
                disabled={createComment.isPending || !commentDraft.trim()}
                className="rounded-lg bg-indigo-600 px-4 py-1.5 text-sm font-medium text-white transition hover:bg-indigo-500 disabled:opacity-50"
              >
                {createComment.isPending ? 'Posting…' : 'Comment'}
              </button>
            </div>
          </form>
        ) : (
          <p className="mt-4 text-sm text-zinc-500 dark:text-zinc-400">
            <Link
              to="/login"
              state={{ from: `/post/${post.id}` }}
              className="font-medium text-indigo-600 hover:underline dark:text-indigo-400"
            >
              Sign in
            </Link>{' '}
            to join the discussion.
          </p>
        )}
      </section>
    </div>
  )
}
