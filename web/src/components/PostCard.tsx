import { MessageSquare } from 'lucide-react'
import { Link } from 'react-router'
import { useVotePost } from '../lib/queries'
import { timeAgo } from '../lib/time'
import type { Post } from '../lib/types'
import VoteButtons from './VoteButtons'

export default function PostCard({ post }: { post: Post }) {
  const vote = useVotePost()

  return (
    <article className="rounded-xl border border-zinc-200 bg-white p-4 transition hover:border-zinc-300 dark:border-zinc-800 dark:bg-zinc-900 dark:hover:border-zinc-700">
      <div className="flex items-center gap-2 text-sm text-zinc-500 dark:text-zinc-400">
        <Link to={`/u/${post.author}`} className="font-medium text-zinc-700 hover:underline dark:text-zinc-300">
          {post.author}
        </Link>
        <span aria-hidden>·</span>
        <time dateTime={post.created_at}>{timeAgo(post.created_at)}</time>
        {post.categories?.map((c) => (
          <Link
            key={c.id}
            to={`/?category=${c.id}`}
            className="rounded-full bg-zinc-100 px-2 py-0.5 text-xs font-medium text-zinc-600 transition hover:bg-zinc-200 dark:bg-zinc-800 dark:text-zinc-300 dark:hover:bg-zinc-700"
          >
            {c.name}
          </Link>
        ))}
      </div>
      <h2 className="mt-1.5 text-lg font-semibold">
        <Link to={`/post/${post.id}`} className="hover:text-indigo-600 dark:hover:text-indigo-400">
          {post.title}
        </Link>
      </h2>
      <p className="mt-1 line-clamp-3 text-sm text-zinc-600 dark:text-zinc-400">{post.content}</p>
      {post.image_url && (
        <Link to={`/post/${post.id}`}>
          <img
            src={post.image_url}
            alt=""
            className="mt-3 max-h-64 w-full rounded-lg border border-zinc-200 object-cover dark:border-zinc-800"
          />
        </Link>
      )}
      {post.tags && post.tags.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-1.5">
          {post.tags.map((t) => (
            <Link
              key={t.id}
              to={`/?tag=${t.name}`}
              className="text-xs font-medium text-indigo-600 hover:underline dark:text-indigo-400"
            >
              #{t.name}
            </Link>
          ))}
        </div>
      )}
      <div className="mt-3 flex items-center gap-3">
        <VoteButtons
          likes={post.likes}
          dislikes={post.dislikes}
          onVote={(isLike) => vote.mutate({ postId: post.id, isLike })}
          disabled={vote.isPending}
        />
        <Link
          to={`/post/${post.id}`}
          className="flex items-center gap-1.5 rounded-full px-2.5 py-1 text-sm text-zinc-500 transition hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
        >
          <MessageSquare size={15} />
          {post.comment_count}
        </Link>
      </div>
    </article>
  )
}
