import { Component, type ErrorInfo, type ReactNode } from 'react'

interface Props {
  children: ReactNode
  /**
   * When this changes the boundary clears its error. Pass the current path, so
   * that a failed page doesn't keep the whole app on the error screen after the
   * user navigates somewhere that would work.
   */
  resetKey?: string
  /** What the Reload button does; a page reload unless a test says otherwise. */
  onReload?: () => void
}

interface State {
  failed: boolean
  /** The resetKey this state was last reconciled with. */
  resetKey?: string
}

/**
 * Catches a page that fails to load or render. The common cause is a stale tab:
 * the app is split into hashed chunks, a deploy replaces them, and a tab opened
 * before it asks for a chunk that no longer exists. A reload fetches the new
 * index.html and with it the new chunk names.
 */
export default class RouteErrorBoundary extends Component<Props, State> {
  state: State = { failed: false, resetKey: this.props.resetKey }

  static getDerivedStateFromError(): Partial<State> {
    return { failed: true }
  }

  static getDerivedStateFromProps(props: Props, state: State): Partial<State> | null {
    // Clear the error when the key changes, without the extra render that
    // setState in componentDidUpdate would cause.
    if (props.resetKey !== state.resetKey) return { failed: false, resetKey: props.resetKey }
    return null
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('page failed to load', error, info.componentStack)
  }

  render() {
    if (!this.state.failed) return this.props.children
    return (
      <div role="alert" className="py-16 text-center">
        <h1 className="text-xl font-bold">Something went wrong loading this page</h1>
        <p className="mt-2 text-zinc-500 dark:text-zinc-400">
          The site may have been updated since you opened it.
        </p>
        <button
          type="button"
          onClick={this.props.onReload ?? (() => window.location.reload())}
          className="mt-4 rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-500"
        >
          Reload
        </button>
      </div>
    )
  }
}
