import { Ban } from 'lucide-react'
import { useIsBlocked, useSetBlocked } from '../lib/queries'

export default function BlockButton({ username }: { username: string }) {
  const isBlocked = useIsBlocked(username)
  const setBlocked = useSetBlocked()

  return (
    <button
      type="button"
      onClick={() => setBlocked.mutate({ username, blocked: !isBlocked })}
      disabled={setBlocked.isPending}
      className="flex items-center gap-1.5 rounded-lg border border-zinc-300 px-2.5 py-1 text-xs font-medium text-zinc-600 transition hover:bg-zinc-100 disabled:opacity-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
    >
      <Ban size={13} />
      {isBlocked ? 'Unblock' : 'Block'}
    </button>
  )
}
