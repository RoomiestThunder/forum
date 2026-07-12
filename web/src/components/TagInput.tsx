import { useState } from 'react'
import { X } from 'lucide-react'
import { useTagSuggestions } from '../lib/queries'

interface Props {
  tags: string[]
  onChange: (tags: string[]) => void
  max?: number
}

export default function TagInput({ tags, onChange, max = 5 }: Props) {
  const [draft, setDraft] = useState('')
  const { data: suggestions } = useTagSuggestions(draft.trim())

  const addTag = (raw: string) => {
    const name = raw.trim().toLowerCase()
    if (!name || tags.includes(name) || tags.length >= max) return
    onChange([...tags, name])
    setDraft('')
  }

  const visibleSuggestions = (suggestions ?? []).filter((t) => !tags.includes(t.name))

  return (
    <div>
      <div className="flex flex-wrap items-center gap-1.5 rounded-lg border border-zinc-300 bg-white px-2 py-1.5 focus-within:border-indigo-500 focus-within:ring-2 focus-within:ring-indigo-500/20 dark:border-zinc-700 dark:bg-zinc-900">
        {tags.map((tag) => (
          <span
            key={tag}
            className="flex items-center gap-1 rounded-full bg-indigo-100 px-2 py-0.5 text-sm text-indigo-700 dark:bg-indigo-950 dark:text-indigo-300"
          >
            #{tag}
            <button
              type="button"
              onClick={() => onChange(tags.filter((t) => t !== tag))}
              aria-label={`Remove tag ${tag}`}
              className="rounded-full hover:text-indigo-900 dark:hover:text-indigo-100"
            >
              <X size={12} />
            </button>
          </span>
        ))}
        {tags.length < max && (
          <input
            type="text"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ',') {
                e.preventDefault()
                addTag(draft)
              } else if (e.key === 'Backspace' && draft === '' && tags.length > 0) {
                onChange(tags.slice(0, -1))
              }
            }}
            placeholder={tags.length === 0 ? 'Add tags…' : ''}
            className="min-w-[6rem] flex-1 bg-transparent px-1 py-0.5 text-sm outline-none"
          />
        )}
      </div>
      {draft.trim() && visibleSuggestions.length > 0 && (
        <div className="mt-1 flex flex-wrap gap-1.5">
          {visibleSuggestions.map((t) => (
            <button
              key={t.id}
              type="button"
              onClick={() => addTag(t.name)}
              className="rounded-full bg-zinc-100 px-2 py-0.5 text-xs text-zinc-600 transition hover:bg-zinc-200 dark:bg-zinc-800 dark:text-zinc-300 dark:hover:bg-zinc-700"
            >
              #{t.name}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
