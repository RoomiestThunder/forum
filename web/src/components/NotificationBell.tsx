import { useEffect, useRef, useState } from 'react'
import { AtSign, Bell, MessageSquare, ThumbsUp } from 'lucide-react'
import { useNavigate } from 'react-router'
import {
  useMarkAllNotificationsRead,
  useMarkNotificationRead,
  useNotifications,
  useUnreadNotificationCount,
} from '../lib/queries'
import { timeAgo } from '../lib/time'
import type { Notification } from '../lib/types'

function describe(n: Notification): string {
  switch (n.type) {
    case 'new_comment':
      return `${n.payload.actor} commented on "${n.payload.post_title}"`
    case 'new_like':
      return `${n.payload.actor} liked "${n.payload.post_title}"`
    case 'new_mention':
      return `${n.payload.actor} mentioned you on "${n.payload.post_title}"`
    default:
      return 'New notification'
  }
}

function postIdFor(n: Notification): number {
  return n.payload.post_id
}

export default function NotificationBell() {
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()

  const { data: count } = useUnreadNotificationCount()
  const { data: notifications } = useNotifications()
  const markRead = useMarkNotificationRead()
  const markAllRead = useMarkAllNotificationsRead()

  useEffect(() => {
    if (!open) return
    const onClickAway = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', onClickAway)
    return () => document.removeEventListener('mousedown', onClickAway)
  }, [open])

  const unread = count?.count ?? 0

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-label={unread > 0 ? `${unread} unread notifications` : 'Notifications'}
        className="relative rounded-lg p-2 text-zinc-500 transition hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
      >
        <Bell size={18} />
        {unread > 0 && (
          <span className="absolute top-1 right-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-red-500 px-1 text-[10px] font-bold text-white">
            {unread > 9 ? '9+' : unread}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 z-20 mt-2 w-80 rounded-xl border border-zinc-200 bg-white shadow-lg dark:border-zinc-800 dark:bg-zinc-900">
          <div className="flex items-center justify-between border-b border-zinc-200 px-3 py-2 dark:border-zinc-800">
            <span className="text-sm font-semibold">Notifications</span>
            {unread > 0 && (
              <button
                type="button"
                onClick={() => markAllRead.mutate()}
                disabled={markAllRead.isPending}
                className="text-xs font-medium text-indigo-600 hover:underline disabled:opacity-50 dark:text-indigo-400"
              >
                Mark all read
              </button>
            )}
          </div>
          <div className="max-h-96 overflow-y-auto">
            {(notifications ?? []).length === 0 && (
              <p className="px-3 py-6 text-center text-sm text-zinc-500 dark:text-zinc-400">
                Nothing here yet.
              </p>
            )}
            {(notifications ?? []).map((n) => (
              <button
                key={n.id}
                type="button"
                onClick={() => {
                  if (!n.read) markRead.mutate(n.id)
                  setOpen(false)
                  navigate(`/post/${postIdFor(n)}`)
                }}
                className={`flex w-full items-start gap-2.5 border-b border-zinc-100 px-3 py-2.5 text-left text-sm transition last:border-0 hover:bg-zinc-50 dark:border-zinc-800/60 dark:hover:bg-zinc-800/50 ${
                  n.read ? 'text-zinc-500 dark:text-zinc-400' : 'font-medium text-zinc-900 dark:text-zinc-100'
                }`}
              >
                {n.type === 'new_comment' ? (
                  <MessageSquare size={15} className="mt-0.5 shrink-0 text-indigo-500" />
                ) : n.type === 'new_mention' ? (
                  <AtSign size={15} className="mt-0.5 shrink-0 text-violet-500" />
                ) : (
                  <ThumbsUp size={15} className="mt-0.5 shrink-0 text-emerald-500" />
                )}
                <span className="flex-1">
                  {describe(n)}
                  <span className="mt-0.5 block text-xs font-normal text-zinc-400">{timeAgo(n.created_at)}</span>
                </span>
                {!n.read && <span className="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-indigo-600" />}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
