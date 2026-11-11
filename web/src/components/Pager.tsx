interface Props {
  page: number
  pageSize: number
  total: number
  onChange: (page: number) => void
}

/** Previous / Next controls for a paginated admin list. Renders nothing for a single page. */
export default function Pager({ page, pageSize, total, onChange }: Props) {
  const pages = Math.max(1, Math.ceil(total / pageSize))
  if (pages <= 1) return null

  const button =
    'rounded-lg border border-zinc-300 px-3 py-1.5 text-sm font-medium text-zinc-600 transition hover:bg-zinc-100 disabled:opacity-40 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800'

  return (
    <nav aria-label="Pagination" className="mt-3 flex items-center justify-center gap-3">
      <button type="button" onClick={() => onChange(page - 1)} disabled={page <= 1} className={button}>
        Previous
      </button>
      <span className="text-sm text-zinc-500 dark:text-zinc-400">
        Page {page} of {pages}
      </span>
      <button type="button" onClick={() => onChange(page + 1)} disabled={page >= pages} className={button}>
        Next
      </button>
    </nav>
  )
}
