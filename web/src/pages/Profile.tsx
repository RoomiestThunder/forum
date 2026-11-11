import { Loader2, Mail, Settings } from 'lucide-react'
import { Link, useParams } from 'react-router'
import BlockButton from '../components/BlockButton'
import PostCard from '../components/PostCard'
import { usePostsFeed, useProfile } from '../lib/queries'
import { fullDate } from '../lib/time'
import { useAuth } from '../stores/auth'

export default function Profile() {
  const { username = '' } = useParams()
  const { data: profile, isLoading, isError } = useProfile(username)
  const feed = usePostsFeed({ author: username })
  const isOwn = useAuth((s) => s.username) === username
  const loggedIn = useAuth((s) => s.refreshToken !== null)

  if (isLoading) {
    return (
      <div className="flex justify-center py-16">
        <Loader2 className="animate-spin text-zinc-400" />
      </div>
    )
  }

  if (isError || !profile) {
    return <p className="py-16 text-center text-zinc-500 dark:text-zinc-400">User not found.</p>
  }

  const posts = feed.data?.pages.flat() ?? []

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-4">
        {profile.avatar_url ? (
          <img
            src={profile.avatar_url}
            alt={profile.username}
            className="h-16 w-16 rounded-full object-cover"
          />
        ) : (
          <div className="flex h-16 w-16 items-center justify-center rounded-full bg-indigo-100 text-xl font-semibold text-indigo-600 dark:bg-indigo-950 dark:text-indigo-300">
            {profile.username.slice(0, 1).toUpperCase()}
          </div>
        )}
        <div className="flex-1">
          <div className="flex items-center gap-2">
            <h1 className="text-xl font-bold">{profile.username}</h1>
            {isOwn && (
              <Link
                to="/settings"
                aria-label="Edit profile"
                className="rounded p-1 text-zinc-400 transition hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-zinc-800 dark:hover:text-zinc-200"
              >
                <Settings size={16} />
              </Link>
            )}
            {!isOwn && loggedIn && (
              <>
                <Link
                  to={`/messages/${profile.username}`}
                  className="flex items-center gap-1.5 rounded-lg border border-zinc-300 px-2.5 py-1 text-xs font-medium text-zinc-600 transition hover:bg-zinc-100 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
                >
                  <Mail size={13} />
                  Message
                </Link>
                <BlockButton username={profile.username} />
              </>
            )}
          </div>
          {profile.bio && <p className="mt-1 text-sm">{profile.bio}</p>}
          <p className="mt-1 text-sm text-zinc-500 dark:text-zinc-400">
            {profile.post_count} {profile.post_count === 1 ? 'post' : 'posts'} · joined{' '}
            {fullDate(profile.created_at)}
          </p>
        </div>
      </div>

      <div className="space-y-4">
        {feed.isSuccess && posts.length === 0 && (
          <p className="py-8 text-center text-zinc-500 dark:text-zinc-400">No posts yet.</p>
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
    </div>
  )
}
