import { useEffect, useRef } from 'react'
import { CheckCircle2, Loader2, XCircle } from 'lucide-react'
import { Link, useSearchParams } from 'react-router'
import { useVerifyEmail } from '../lib/queries'

export default function VerifyEmail() {
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') ?? ''
  const verifyEmail = useVerifyEmail()
  const submitted = useRef(false)

  useEffect(() => {
    if (token && !submitted.current) {
      submitted.current = true
      verifyEmail.mutate(token)
    }
  }, [token, verifyEmail])

  if (!token) {
    return (
      <div className="mx-auto mt-16 w-full max-w-sm text-center">
        <XCircle className="mx-auto mb-2 text-red-500" size={32} />
        <h1 className="text-2xl font-bold">Invalid link</h1>
        <p className="mt-2 text-sm text-zinc-500 dark:text-zinc-400">This verification link is missing its token.</p>
      </div>
    )
  }

  return (
    <div className="mx-auto mt-16 w-full max-w-sm text-center">
      {verifyEmail.isPending && (
        <>
          <Loader2 className="mx-auto mb-2 animate-spin text-zinc-400" size={32} />
          <p className="text-sm text-zinc-500 dark:text-zinc-400">Verifying your email…</p>
        </>
      )}
      {verifyEmail.isSuccess && (
        <>
          <CheckCircle2 className="mx-auto mb-2 text-emerald-500" size={32} />
          <h1 className="text-2xl font-bold">Email verified</h1>
          <Link to="/" className="mt-4 inline-block text-sm font-medium text-indigo-600 hover:underline dark:text-indigo-400">
            Back to the feed
          </Link>
        </>
      )}
      {verifyEmail.isError && (
        <>
          <XCircle className="mx-auto mb-2 text-red-500" size={32} />
          <h1 className="text-2xl font-bold">Verification failed</h1>
          <p className="mt-2 text-sm text-zinc-500 dark:text-zinc-400">{verifyEmail.error.message}</p>
        </>
      )}
    </div>
  )
}
