import { Mail } from 'lucide-react'
import { Link } from 'react-router'
import { useUnreadMessageCount } from '../lib/queries'

export default function MessagesIcon() {
  const { data } = useUnreadMessageCount()
  const unread = data?.count ?? 0

  return (
    <Link
      to="/messages"
      aria-label={unread > 0 ? `${unread} unread messages` : 'Messages'}
      className="relative rounded-lg p-2 text-zinc-500 transition hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
    >
      <Mail size={18} />
      {unread > 0 && (
        <span className="absolute top-1 right-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-red-500 px-1 text-[10px] font-bold text-white">
          {unread > 9 ? '9+' : unread}
        </span>
      )}
    </Link>
  )
}
