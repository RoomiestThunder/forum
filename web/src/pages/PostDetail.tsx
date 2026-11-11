import { useMemo } from 'react'
import { Bookmark, Loader2, Pencil, Pin, Send, Trash2 } from 'lucide-react'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Link, useNavigate, useParams } from 'react-router'
import CommentForm from '../components/CommentForm'
import CommentThread from '../components/CommentThread'
import ReportButton from '../components/ReportButton'
import VoteButtons from '../components/VoteButtons'
import {
  useAdminDeletePost,
  useDeletePost,
  useMe,
  usePost,
  usePublishPost,
  useSetPostPinned,
  useToggleBookmark,
  useVotePost,
} from '../lib/queries'
import { buildCommentTree, countLive } from '../lib/commentTree'
import { fullDate, timeAgo } from '../lib/time'
import { useAuth } from '../stores/auth'

export default function PostDetail() {
  const { id } = useParams()
  const postId = Number(id)
  const navigate = useNavigate()

  const userId = useAuth((s) => s.userId)
  const loggedIn = useAuth((s) => s.refreshToken !== null)
  const { data: me } = useMe()
  const isStaff = !!me && (me.is_admin || me.is_moderator)

  const { data, isLoading, isError, error } = usePost(postId)
  const vote = useVotePost()
  const deletePost = useDeletePost()
  const adminDeletePost = useAdminDeletePost()
  const setPinned = useSetPostPinned()
  const toggleBookmark = useToggleBookmark(postId)
  const publishPost = usePublishPost()

  const commentList = data?.comments
  const commentTree = useMemo(() => buildCommentTree(commentList ?? []), [commentList])

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
          {post.draft && (
            <span className="rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-950/50 dark:text-amber-400">
              Draft
            </span>
          )}
          {(isOwn || isStaff) && (
            <span className="ml-auto flex items-center gap-1">
              {isOwn && post.draft && (
                <button
                  type="button"
                  onClick={() => publishPost.mutate(post.id)}
                  disabled={publishPost.isPending}
                  className="flex items-center gap-1.5 rounded-lg bg-indigo-600 px-2.5 py-1 text-xs font-medium text-white transition hover:bg-indigo-500 disabled:opacity-50"
                >
                  <Send size={13} />
                  {publishPost.isPending ? 'Publishing…' : 'Publish'}
                </button>
              )}
              {isStaff && (
                <button
                  type="button"
                  onClick={() => setPinned.mutate({ postId: post.id, pinned: !post.pinned })}
                  disabled={setPinned.isPending}
                  aria-label={post.pinned ? 'Unpin post' : 'Pin post'}
                  aria-pressed={post.pinned}
                  className={`rounded p-1.5 transition disabled:opacity-50 ${
                    post.pinned
                      ? 'text-indigo-600 dark:text-indigo-400'
                      : 'text-zinc-400 hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200'
                  }`}
                >
                  <Pin size={16} fill={post.pinned ? 'currentColor' : 'none'} />
                </button>
              )}
              {isOwn && (
                <Link
                  to={`/post/${post.id}/edit`}
                  aria-label="Edit post"
                  className="rounded p-1.5 text-zinc-400 transition hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
                >
                  <Pencil size={16} />
                </Link>
              )}
              <button
                type="button"
                onClick={() => {
                  if (window.confirm('Delete this post?')) {
                    if (isOwn) {
                      deletePost.mutate(post.id, { onSuccess: () => navigate('/') })
                    } else {
                      adminDeletePost.mutate(post.id, { onSuccess: () => navigate('/') })
                    }
                  }
                }}
                disabled={deletePost.isPending || adminDeletePost.isPending}
                aria-label="Delete post"
                className="rounded p-1.5 text-zinc-400 transition hover:bg-zinc-100 hover:text-red-600 disabled:opacity-50 dark:hover:bg-zinc-800 dark:hover:text-red-400"
              >
                <Trash2 size={16} />
              </button>
            </span>
          )}
        </div>

        <h1 className="mt-2 flex items-center gap-2 text-2xl font-bold">
          {post.pinned && <Pin size={18} className="shrink-0 text-indigo-500" fill="currentColor" />}
          {post.title}
        </h1>

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
          {loggedIn && !isOwn && <ReportButton targetType="post" targetId={post.id} className="ml-auto" />}
        </div>
      </article>

      <section>
        <h2 className="mb-3 text-lg font-semibold">
          Comments <span className="text-zinc-400">({countLive(comments)})</span>
        </h2>

        <div className="space-y-2">
          {commentTree.map((node) => (
            <CommentThread key={node.comment.id} node={node} postId={postId} />
          ))}
          {comments.length === 0 && (
            <p className="py-4 text-sm text-zinc-500 dark:text-zinc-400">No comments yet.</p>
          )}
        </div>

        {loggedIn ? (
          <div className="mt-4">
            <CommentForm postId={postId} />
          </div>
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
