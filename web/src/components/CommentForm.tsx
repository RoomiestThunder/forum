import { useState } from 'react'
import { useCreateComment } from '../lib/queries'
import MentionTextarea from './MentionTextarea'

interface Props {
  postId: number
  /** Set to post a reply to that comment instead of a top-level comment. */
  parentId?: number
  placeholder?: string
  submitLabel?: string
  autoFocus?: boolean
  /** Called after the comment was posted. */
  onDone?: () => void
  /** When given, a Cancel button is shown. */
  onCancel?: () => void
}

const MAX_LENGTH = 500

/** The compose box for a top-level comment or a reply. */
export default function CommentForm({
  postId,
  parentId,
  placeholder = 'Write a comment…',
  submitLabel = 'Comment',
  autoFocus,
  onDone,
  onCancel,
}: Props) {
  const [draft, setDraft] = useState('')
  const create = useCreateComment(postId)

  const submit = () => {
    const content = draft.trim()
    if (!content) return
    create.mutate(
      { content, parentId },
      {
        onSuccess: () => {
          setDraft('')
          onDone?.()
        },
      },
    )
  }

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      {create.isError && (
        <p
          role="alert"
          className="mb-2 rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950/50 dark:text-red-300"
        >
          {create.error.message}
        </p>
      )}
      <MentionTextarea
        value={draft}
        onChange={setDraft}
        placeholder={placeholder}
        maxLength={MAX_LENGTH}
        rows={3}
        autoFocus={autoFocus}
        className="w-full rounded-lg border border-zinc-300 bg-white px-3 py-2 text-sm outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-zinc-700 dark:bg-zinc-900"
      />
      <div className="mt-2 flex items-center justify-between">
        <span className="text-xs text-zinc-400">
          {draft.length}/{MAX_LENGTH}
        </span>
        <div className="flex gap-2">
          {onCancel && (
            <button
              type="button"
              onClick={onCancel}
              className="rounded-lg px-3 py-1.5 text-sm font-medium text-zinc-600 transition hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800"
            >
              Cancel
            </button>
          )}
          <button
            type="submit"
            disabled={create.isPending || !draft.trim()}
            className="rounded-lg bg-indigo-600 px-4 py-1.5 text-sm font-medium text-white transition hover:bg-indigo-500 disabled:opacity-50"
          >
            {create.isPending ? 'Posting…' : submitLabel}
          </button>
        </div>
      </div>
    </form>
  )
}
