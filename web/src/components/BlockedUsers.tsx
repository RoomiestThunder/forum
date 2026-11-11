import { Link } from 'react-router'
import { useBlockedUsers, useSetBlocked } from '../lib/queries'

/** The signed-in user's block list, with a way to undo each block. */
export default function BlockedUsers() {
  const { data: blocked, isLoading } = useBlockedUsers()
  const setBlocked = useSetBlocked()

  return (
    <section className="mt-10" aria-labelledby="blocked-users-heading">
      <h2 id="blocked-users-heading" className="text-lg font-semibold">
        Blocked users
      </h2>
      <p className="mt-1 text-sm text-zinc-500 dark:text-zinc-400">
        Blocked users can't message you, and their replies, mentions and likes won't notify you.
      </p>

      {isLoading && <p className="mt-3 text-sm text-zinc-500">Loading…</p>}
      {!isLoading && blocked?.length === 0 && (
        <p className="mt-3 text-sm text-zinc-500 dark:text-zinc-400">You haven't blocked anyone.</p>
      )}

      <ul className="mt-3 space-y-2">
        {blocked?.map((user) => (
          <li
            key={user.id}
            className="flex items-center justify-between rounded-lg border border-zinc-200 px-3 py-2 dark:border-zinc-800"
          >
            <Link to={`/u/${user.username}`} className="text-sm font-medium hover:underline">
              {user.username}
            </Link>
            <button
              type="button"
              onClick={() => setBlocked.mutate({ username: user.username, blocked: false })}
              disabled={setBlocked.isPending}
              aria-label={`Unblock ${user.username}`}
              className="rounded-lg px-2.5 py-1 text-xs font-medium text-zinc-600 transition hover:bg-zinc-100 disabled:opacity-50 dark:text-zinc-300 dark:hover:bg-zinc-800"
            >
              Unblock
            </button>
          </li>
        ))}
      </ul>
    </section>
  )
}
