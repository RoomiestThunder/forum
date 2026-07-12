import { useState } from 'react'
import { Flag } from 'lucide-react'
import { useCreateReport } from '../lib/queries'

interface Props {
  targetType: 'post' | 'comment'
  targetId: number
  className?: string
}

export default function ReportButton({ targetType, targetId, className }: Props) {
  const [open, setOpen] = useState(false)
  const [reason, setReason] = useState('')
  const report = useCreateReport()

  if (report.isSuccess) {
    return <span className={`text-xs text-zinc-400 ${className ?? ''}`}>Reported</span>
  }

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        aria-label={`Report this ${targetType}`}
        className={`flex items-center gap-1 text-zinc-400 transition hover:text-red-500 ${className ?? ''}`}
      >
        <Flag size={13} />
      </button>
    )
  }

  return (
    <div className="mt-2 w-full max-w-sm rounded-lg border border-zinc-200 bg-white p-3 text-sm shadow-sm dark:border-zinc-800 dark:bg-zinc-900">
      <p className="mb-1.5 font-medium">Report this {targetType}</p>
      <textarea
        value={reason}
        onChange={(e) => setReason(e.target.value)}
        placeholder="Why are you reporting this?"
        rows={2}
        maxLength={500}
        autoFocus
        className="w-full rounded-lg border border-zinc-300 bg-white px-2 py-1.5 text-sm outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-zinc-700 dark:bg-zinc-950"
      />
      {report.isError && <p className="mt-1 text-xs text-red-600 dark:text-red-400">{report.error.message}</p>}
      <div className="mt-2 flex gap-2">
        <button
          type="button"
          onClick={() =>
            report.mutate({ target_type: targetType, target_id: targetId, reason: reason.trim() })
          }
          disabled={report.isPending || !reason.trim()}
          className="rounded-lg bg-red-600 px-3 py-1 text-xs font-medium text-white transition hover:bg-red-500 disabled:opacity-50"
        >
          {report.isPending ? 'Submitting…' : 'Submit'}
        </button>
        <button
          type="button"
          onClick={() => setOpen(false)}
          className="rounded-lg px-3 py-1 text-xs font-medium text-zinc-600 transition hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800"
        >
          Cancel
        </button>
      </div>
    </div>
  )
}
