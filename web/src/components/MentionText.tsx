import type { ReactNode } from 'react'
import { Link } from 'react-router'

const MENTION_RE = /@([a-zA-Z0-9_]{3,20})/g

/** Renders text with @mentions turned into profile links. */
export default function MentionText({ text }: { text: string }) {
  const parts: ReactNode[] = []
  let lastIndex = 0
  MENTION_RE.lastIndex = 0
  let match: RegExpExecArray | null
  while ((match = MENTION_RE.exec(text))) {
    if (match.index > lastIndex) parts.push(text.slice(lastIndex, match.index))
    parts.push(
      <Link
        key={match.index}
        to={`/u/${match[1]}`}
        className="font-medium text-indigo-600 hover:underline dark:text-indigo-400"
      >
        @{match[1]}
      </Link>,
    )
    lastIndex = match.index + match[0].length
  }
  if (lastIndex < text.length) parts.push(text.slice(lastIndex))
  return <>{parts}</>
}
