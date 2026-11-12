import { Loader2 } from 'lucide-react'

/** Shown in place of a page while its code is still downloading. */
export default function PageSpinner() {
  return (
    <div role="status" aria-label="Loading" className="flex justify-center py-16">
      <Loader2 className="animate-spin text-zinc-400" />
    </div>
  )
}
