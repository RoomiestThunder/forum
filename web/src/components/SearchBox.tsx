import { useEffect, useState } from 'react'
import { Search } from 'lucide-react'
import { useLocation, useNavigate, useSearchParams } from 'react-router'
import { useDebounce } from '../lib/useDebounce'

export default function SearchBox() {
  const navigate = useNavigate()
  const location = useLocation()
  const [searchParams] = useSearchParams()
  const onSearchPage = location.pathname === '/search'

  const [value, setValue] = useState(onSearchPage ? (searchParams.get('q') ?? '') : '')
  const debounced = useDebounce(value, 300)

  // Live-update the URL as the user types, replacing history entries so
  // back/forward doesn't step through every keystroke.
  useEffect(() => {
    if (!onSearchPage) return
    const next = new URLSearchParams(searchParams)
    if (debounced.trim()) next.set('q', debounced)
    else next.delete('q')
    next.delete('page')
    navigate(`/search?${next}`, { replace: true })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debounced, onSearchPage])

  return (
    <div className="flex-1">
      <form
        onSubmit={(e) => {
          e.preventDefault()
          if (value.trim()) navigate(`/search?q=${encodeURIComponent(value)}`)
        }}
        className="relative hidden max-w-xs md:block"
      >
        <Search size={15} className="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2 text-zinc-400" />
        <input
          type="search"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder="Search posts…"
          className="w-full rounded-lg border border-zinc-300 bg-white py-1.5 pr-3 pl-8 text-sm outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-zinc-700 dark:bg-zinc-900"
        />
      </form>
    </div>
  )
}
