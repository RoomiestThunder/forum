import type { ReactElement } from 'react'
import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'

export function renderWithProviders(ui: ReactElement, { route = '/' }: { route?: string } = {}) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[route]}>{ui}</MemoryRouter>
    </QueryClientProvider>,
  )
}

type Handler = unknown | ((init?: RequestInit) => unknown)

/**
 * Builds an implementation for a mocked `api()` that answers by "METHOD /path",
 * e.g. 'GET /blocks' or 'PUT /blocks/bob' (spell GET out). A value is returned
 * as the response; a function is called with the request init and may throw to
 * simulate an error. An unlisted route fails the test loudly instead of
 * silently returning undefined, so a component quietly calling something the
 * test didn't anticipate can't slip through.
 */
export function routeApi(routes: Record<string, Handler>) {
  return async (path: string, init: RequestInit = {}) => {
    const key = `${init.method ?? 'GET'} ${path}`
    if (!(key in routes)) throw new Error(`unexpected api call: ${key}`)
    const handler = routes[key]
    return typeof handler === 'function' ? handler(init) : handler
  }
}
