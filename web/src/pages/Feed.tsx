import { Loader2 } from 'lucide-react'
import { Link, useSearchParams } from 'react-router'
import PostCard from '../components/PostCard'
import { useCategories, usePostsFeed, type FeedParams } from '../lib/queries'
import { useAuth } from '../stores/auth'

const FILTERS = [
  { key: undefined, label: 'Latest' },
  { key: 'myposts', label: 'My posts' },
  { key: 'liked', label: 'Liked' },
  { key: 'saved', label: 'Saved' },
] as const

export default function Feed() {
  const [searchParams] = useSearchParams()
  const loggedIn = useAuth((s) => s.refreshToken !== null)

  const filterParam = searchParams.get('filter')
  const categoryParam = searchParams.get('category')
  const tagParam = searchParams.get('tag')

  const params: FeedParams = {}
  if (filterParam === 'myposts' || filterParam === 'liked' || filterParam === 'saved') {
    params.filter = filterParam
  }
  if (categoryParam) params.category = Number(categoryParam)
  if (tagParam) params.tag = tagParam

  const feed = usePostsFeed(params)
  const { data: categories } = useCategories()

  const posts = feed.data?.pages.flat() ?? []

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        {loggedIn &&
          FILTERS.map(({ key, label }) => {
            const active = params.filter === key || (!params.filter && key === undefined)
            return (
              <Link
                key={label}
                to={key ? `/?filter=${key}` : '/'}
                className={`rounded-full px-3 py-1 text-sm font-medium transition ${
                  active && !params.category && !params.tag
                    ? 'bg-indigo-600 text-white'
                    : 'bg-zinc-100 text-zinc-600 hover:bg-zinc-200 dark:bg-zinc-800 dark:text-zinc-300 dark:hover:bg-zinc-700'
                }`}
              >
                {label}
              </Link>
            )
          })}
        {(categories ?? []).map((c) => (
          <Link
            key={c.id}
            to={`/?category=${c.id}`}
            className={`rounded-full px-3 py-1 text-sm font-medium transition ${
              params.category === c.id
                ? 'bg-indigo-600 text-white'
                : 'bg-zinc-100 text-zinc-600 hover:bg-zinc-200 dark:bg-zinc-800 dark:text-zinc-300 dark:hover:bg-zinc-700'
            }`}
          >
            {c.name}
          </Link>
        ))}
        {params.tag && (
          <span className="flex items-center gap-1.5 rounded-full bg-indigo-600 px-3 py-1 text-sm font-medium text-white">
            #{params.tag}
            <Link to="/" aria-label="Clear tag filter" className="hover:opacity-75">
              ×
            </Link>
          </span>
        )}
      </div>

      {feed.isLoading && (
        <div className="flex justify-center py-16">
          <Loader2 className="animate-spin text-zinc-400" />
        </div>
      )}

      {feed.isError && (
        <p className="rounded-lg bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/50 dark:text-red-300">
          Failed to load posts: {feed.error.message}
        </p>
      )}

      {feed.isSuccess && posts.length === 0 && (
        <div className="py-16 text-center text-zinc-500 dark:text-zinc-400">
          <p>No posts here yet.</p>
          {loggedIn && (
            <Link to="/new" className="mt-2 inline-block font-medium text-indigo-600 hover:underline dark:text-indigo-400">
              Write the first one
            </Link>
          )}
        </div>
      )}

      {posts.map((post) => (
        <PostCard key={post.id} post={post} />
      ))}

      {feed.hasNextPage && (
        <div className="flex justify-center pt-2">
          <button
            type="button"
            onClick={() => feed.fetchNextPage()}
            disabled={feed.isFetchingNextPage}
            className="rounded-lg border border-zinc-300 px-4 py-2 text-sm font-medium text-zinc-600 transition hover:bg-zinc-100 disabled:opacity-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
          >
            {feed.isFetchingNextPage ? 'Loading…' : 'Load more'}
          </button>
        </div>
      )}
    </div>
  )
}
