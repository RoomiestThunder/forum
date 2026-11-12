import { Suspense, lazy } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import RouteErrorBoundary from './RouteErrorBoundary'

function Boom(): never {
  throw new Error('render failed')
}

beforeEach(() => {
  // React and the boundary both log a caught error; that is expected here.
  vi.spyOn(console, 'error').mockImplementation(() => {})
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('RouteErrorBoundary', () => {
  it('renders its children when nothing fails', () => {
    render(
      <RouteErrorBoundary>
        <p>all good</p>
      </RouteErrorBoundary>,
    )
    expect(screen.getByText('all good')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('shows a message and a Reload button when a child throws', async () => {
    const onReload = vi.fn()
    render(
      <RouteErrorBoundary onReload={onReload}>
        <Boom />
      </RouteErrorBoundary>,
    )

    expect(screen.getByRole('alert')).toHaveTextContent('Something went wrong loading this page')
    await userEvent.setup().click(screen.getByRole('button', { name: 'Reload' }))
    expect(onReload).toHaveBeenCalledTimes(1)
  })

  // What a stale tab hits after a deploy: the hashed chunk it asks for is gone.
  it('catches a page whose code fails to download', async () => {
    const Missing = lazy(() => Promise.reject(new Error('Failed to fetch dynamically imported module')))

    render(
      <RouteErrorBoundary>
        <Suspense fallback={<p>loading…</p>}>
          <Missing />
        </Suspense>
      </RouteErrorBoundary>,
    )

    expect(await screen.findByRole('alert')).toBeInTheDocument()
  })

  it('clears the error when the key changes, so navigating away recovers', () => {
    const { rerender } = render(
      <RouteErrorBoundary resetKey="/broken">
        <Boom />
      </RouteErrorBoundary>,
    )
    expect(screen.getByRole('alert')).toBeInTheDocument()

    rerender(
      <RouteErrorBoundary resetKey="/fine">
        <p>a page that works</p>
      </RouteErrorBoundary>,
    )

    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByText('a page that works')).toBeInTheDocument()
  })

  it('keeps showing the error while the key stays the same', () => {
    const { rerender } = render(
      <RouteErrorBoundary resetKey="/broken">
        <Boom />
      </RouteErrorBoundary>,
    )
    rerender(
      <RouteErrorBoundary resetKey="/broken">
        <p>should not appear</p>
      </RouteErrorBoundary>,
    )

    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(screen.queryByText('should not appear')).not.toBeInTheDocument()
  })
})
