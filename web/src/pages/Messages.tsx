import { Loader2, MessagesSquare } from 'lucide-react'
import { Link } from 'react-router'
import { useConversations } from '../lib/queries'
import { timeAgo } from '../lib/time'

export default function Messages() {
  const { data: conversations, isLoading } = useConversations()

  if (isLoading) {
    return (
      <div className="flex justify-center py-16">
        <Loader2 className="animate-spin text-zinc-400" />
      </div>
    )
  }

  const convos = conversations ?? []

  if (convos.length === 0) {
    return (
      <div className="py-16 text-center text-zinc-500 dark:text-zinc-400">
        <MessagesSquare className="mx-auto mb-3 text-zinc-300 dark:text-zinc-700" size={32} />
        No conversations yet. Visit a profile and hit "Message" to start one.
      </div>
    )
  }

  return (
    <div className="space-y-1">
      <h1 className="mb-4 text-2xl font-bold">Messages</h1>
      {convos.map((c) => (
        <Link
          key={c.id}
          to={`/messages/${c.other_username}`}
          className="flex items-center gap-3 rounded-lg px-3 py-2.5 transition hover:bg-zinc-100 dark:hover:bg-zinc-800"
        >
          {c.other_avatar_url ? (
            <img src={c.other_avatar_url} alt={c.other_username} className="h-10 w-10 shrink-0 rounded-full object-cover" />
          ) : (
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-indigo-100 text-sm font-semibold text-indigo-600 dark:bg-indigo-950 dark:text-indigo-300">
              {c.other_username.slice(0, 1).toUpperCase()}
            </div>
          )}
          <div className="min-w-0 flex-1">
            <div className="flex items-center justify-between gap-2">
              <span className={`truncate ${c.unread_count > 0 ? 'font-semibold' : 'font-medium'}`}>
                {c.other_username}
              </span>
              <span className="shrink-0 text-xs text-zinc-400">{timeAgo(c.last_message_at)}</span>
            </div>
            <p
              className={`truncate text-sm ${
                c.unread_count > 0 ? 'text-zinc-900 dark:text-zinc-100' : 'text-zinc-500 dark:text-zinc-400'
              }`}
            >
              {c.last_message}
            </p>
          </div>
          {c.unread_count > 0 && (
            <span className="flex h-5 min-w-5 shrink-0 items-center justify-center rounded-full bg-indigo-600 px-1.5 text-xs font-bold text-white">
              {c.unread_count > 9 ? '9+' : c.unread_count}
            </span>
          )}
        </Link>
      ))}
    </div>
  )
}
