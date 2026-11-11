import { useEffect, useRef, useState } from 'react'
import { ArrowLeft, Loader2, Send } from 'lucide-react'
import { Link, useParams } from 'react-router'
import { useIsBlocked, useSendMessage, useThread } from '../lib/queries'
import { timeAgo } from '../lib/time'
import { useAuth } from '../stores/auth'

export default function MessageThread() {
  const { username = '' } = useParams()
  const myUsername = useAuth((s) => s.username)
  const { data, isLoading } = useThread(username)
  const send = useSendMessage(username)
  const isBlocked = useIsBlocked(username)
  const [content, setContent] = useState('')
  const bottomRef = useRef<HTMLDivElement>(null)

  const messages = data?.messages ?? []

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ block: 'end' })
  }, [messages.length])

  const submit = () => {
    const trimmed = content.trim()
    if (!trimmed) return
    send.mutate(trimmed, { onSuccess: () => setContent('') })
  }

  return (
    <div className="flex h-[calc(100vh-8.5rem)] flex-col">
      <div className="mb-4 flex items-center gap-2 border-b border-zinc-200 pb-3 dark:border-zinc-800">
        <Link
          to="/messages"
          aria-label="Back to messages"
          className="rounded-lg p-1.5 text-zinc-500 transition hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
        >
          <ArrowLeft size={18} />
        </Link>
        <Link to={`/u/${username}`} className="font-semibold hover:underline">
          {username}
        </Link>
      </div>

      <div className="flex-1 space-y-2 overflow-y-auto px-1">
        {isLoading && (
          <div className="flex justify-center py-8">
            <Loader2 className="animate-spin text-zinc-400" />
          </div>
        )}
        {!isLoading && messages.length === 0 && (
          <p className="py-8 text-center text-sm text-zinc-500 dark:text-zinc-400">
            No messages yet. Say hi.
          </p>
        )}
        {messages.map((m) => {
          const mine = m.sender === myUsername
          return (
            <div key={m.id} className={`flex ${mine ? 'justify-end' : 'justify-start'}`}>
              <div
                className={`max-w-[75%] rounded-2xl px-3.5 py-2 text-sm ${
                  mine
                    ? 'rounded-br-sm bg-indigo-600 text-white'
                    : 'rounded-bl-sm bg-zinc-100 text-zinc-900 dark:bg-zinc-800 dark:text-zinc-100'
                }`}
              >
                <p className="whitespace-pre-wrap break-words">{m.content}</p>
                <p className={`mt-0.5 text-[10px] ${mine ? 'text-indigo-200' : 'text-zinc-400'}`}>
                  {timeAgo(m.created_at)}
                </p>
              </div>
            </div>
          )
        })}
        <div ref={bottomRef} />
      </div>

      {isBlocked && (
        <p className="mt-3 text-center text-sm text-zinc-500 dark:text-zinc-400">
          You blocked {username}. Unblock them from their{' '}
          <Link to={`/u/${username}`} className="underline">
            profile
          </Link>{' '}
          to send messages.
        </p>
      )}
      {send.isError && (
        <p role="alert" className="mt-3 text-sm text-red-600 dark:text-red-400">
          {send.error.message}
        </p>
      )}

      <form
        className="mt-3 flex gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <textarea
          value={content}
          onChange={(e) => setContent(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault()
              submit()
            }
          }}
          placeholder="Write a message…"
          rows={1}
          maxLength={2000}
          disabled={isBlocked}
          className="flex-1 resize-none disabled:opacity-50 rounded-lg border border-zinc-300 bg-white px-3 py-2 text-sm outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-zinc-700 dark:bg-zinc-900"
        />
        <button
          type="submit"
          disabled={isBlocked || send.isPending || content.trim() === ''}
          aria-label="Send"
          className="flex shrink-0 items-center justify-center rounded-lg bg-indigo-600 px-3.5 text-white transition hover:bg-indigo-500 disabled:opacity-50"
        >
          <Send size={16} />
        </button>
      </form>
    </div>
  )
}
