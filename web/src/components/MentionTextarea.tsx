import { useMemo, useState } from 'react'
import { useSearchUsers } from '../lib/queries'

interface Props {
  value: string
  onChange: (value: string) => void
  onKeyDown?: (e: React.KeyboardEvent<HTMLTextAreaElement>) => void
  placeholder?: string
  maxLength?: number
  rows?: number
  className?: string
  autoFocus?: boolean
}

/**
 * A textarea with @mention autocomplete. Only triggers on the token at the
 * very end of the text (not mid-string), which covers the common case of
 * typing a mention while composing without needing full caret-position
 * tracking on a controlled input.
 */
export default function MentionTextarea({
  value,
  onChange,
  onKeyDown,
  placeholder,
  maxLength,
  rows = 3,
  className,
  autoFocus,
}: Props) {
  const [focused, setFocused] = useState(false)

  const mention = useMemo(() => {
    const m = /(?:^|\s)@([a-zA-Z0-9_]{1,20})$/.exec(value)
    if (!m) return null
    return { query: m[1], start: value.length - (m[1].length + 1) }
  }, [value])

  const { data: suggestions } = useSearchUsers(mention?.query ?? '')
  const visible = focused && mention !== null && (suggestions?.length ?? 0) > 0

  const pick = (username: string) => {
    if (!mention) return
    onChange(value.slice(0, mention.start) + '@' + username + ' ')
  }

  return (
    <div className="relative">
      <textarea
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={onKeyDown}
        onFocus={() => setFocused(true)}
        onBlur={() => setTimeout(() => setFocused(false), 150)}
        placeholder={placeholder}
        maxLength={maxLength}
        rows={rows}
        autoFocus={autoFocus}
        className={className}
      />
      {visible && (
        <div className="absolute z-10 mt-1 w-56 overflow-hidden rounded-lg border border-zinc-200 bg-white py-1 shadow-lg dark:border-zinc-800 dark:bg-zinc-900">
          {suggestions!.map((u) => (
            <button
              key={u.id}
              type="button"
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => pick(u.username)}
              className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm hover:bg-zinc-100 dark:hover:bg-zinc-800"
            >
              {u.avatar_url ? (
                <img src={u.avatar_url} alt="" className="h-5 w-5 rounded-full object-cover" />
              ) : (
                <div className="flex h-5 w-5 items-center justify-center rounded-full bg-indigo-100 text-[10px] font-semibold text-indigo-600 dark:bg-indigo-950 dark:text-indigo-300">
                  {u.username.slice(0, 1).toUpperCase()}
                </div>
              )}
              @{u.username}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
