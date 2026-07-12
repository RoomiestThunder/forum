import { Loader2, Search as SearchIcon, SearchX } from 'lucide-react'
import { useSearchParams } from 'react-router'
import PostCard from '../components/PostCard'
import { useSearch } from '../lib/queries'

const PAGE_SIZE = 10

export default function Search() {
  const [searchParams, setSearchParams] = useSearchParams()
  const query = searchParams.get('q') ?? ''
  const page = Number(searchParams.get('page') ?? '1')

  const { data, isLoading, isFetching, isError } = useSearch(query, page)

  const goToPage = (p: number) => {
    const next = new URLSearchParams(searchParams)
    next.set('page', String(p))
    setSearchParams(next)
  }

  const submitQuery = (q: string) => {
    const next = new URLSearchParams(searchParams)
    if (q.trim()) next.set('q', q)
    else next.delete('q')
    next.delete('page')
    setSearchParams(next)
  }

  const totalPages = data ? Math.max(1, Math.ceil(data.total / PAGE_SIZE)) : 1
  const posts = data?.posts ?? []

  return (
    <div className="space-y-4">
      <form
        onSubmit={(e) => {
          e.preventDefault()
          submitQuery(new FormData(e.currentTarget).get('q') as string)
        }}
        className="relative md:hidden"
      >
        <SearchIcon size={15} className="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2 text-zinc-400" />
        <input
          name="q"
          type="search"
          defaultValue={query}
          key={query}
          placeholder="Search posts…"
          className="w-full rounded-lg border border-zinc-300 bg-white py-2 pr-3 pl-8 text-sm outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-zinc-700 dark:bg-zinc-900"
        />
      </form>

      {query.trim() && (
        <h1 className="text-xl font-bold">
          Results for <span className="text-indigo-600 dark:text-indigo-400">&ldquo;{query}&rdquo;</span>
          {data && <span className="ml-2 text-sm font-normal text-zinc-500 dark:text-zinc-400">{data.total} found</span>}
        </h1>
      )}

      {!query.trim() ? (
        <div className="py-16 text-center text-zinc-500 dark:text-zinc-400">
          <SearchX className="mx-auto mb-2" />
          Type something to search.
        </div>
      ) : (
        <>
          {isLoading && (
            <div className="flex justify-center py-16">
              <Loader2 className="animate-spin text-zinc-400" />
            </div>
          )}

          {isError && (
            <p className="rounded-lg bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/50 dark:text-red-300">
              Search failed. Try again.
            </p>
          )}

          {!isLoading && posts.length === 0 && (
            <p className="py-16 text-center text-zinc-500 dark:text-zinc-400">No posts match your search.</p>
          )}

          <div className={isFetching ? 'opacity-60 transition' : 'transition'}>
            {posts.map((post) => (
              <div key={post.id} className="mb-4">
                <PostCard post={post} />
              </div>
            ))}
          </div>

          {totalPages > 1 && (
            <div className="flex items-center justify-center gap-2 pt-2">
              <button
                type="button"
                onClick={() => goToPage(page - 1)}
                disabled={page <= 1}
                className="rounded-lg border border-zinc-300 px-3 py-1.5 text-sm font-medium text-zinc-600 transition hover:bg-zinc-100 disabled:opacity-40 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
              >
                Previous
              </button>
              <span className="text-sm text-zinc-500 dark:text-zinc-400">
                Page {page} of {totalPages}
              </span>
              <button
                type="button"
                onClick={() => goToPage(page + 1)}
                disabled={page >= totalPages}
                className="rounded-lg border border-zinc-300 px-3 py-1.5 text-sm font-medium text-zinc-600 transition hover:bg-zinc-100 disabled:opacity-40 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
              >
                Next
              </button>
            </div>
          )}
        </>
      )}
    </div>
  )
}
