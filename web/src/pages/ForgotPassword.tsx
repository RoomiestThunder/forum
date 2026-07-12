import { useState } from 'react'
import { Link } from 'react-router'
import { useForgotPassword } from '../lib/queries'

export default function ForgotPassword() {
  const [email, setEmail] = useState('')
  const forgotPassword = useForgotPassword()

  if (forgotPassword.isSuccess) {
    return (
      <div className="mx-auto mt-16 w-full max-w-sm text-center">
        <h1 className="text-2xl font-bold">Check your email</h1>
        <p className="mt-3 text-sm text-zinc-500 dark:text-zinc-400">
          If an account exists for <strong>{email}</strong>, a password reset link is on its way.
        </p>
        <Link to="/login" className="mt-4 inline-block text-sm font-medium text-indigo-600 hover:underline dark:text-indigo-400">
          Back to sign in
        </Link>
      </div>
    )
  }

  return (
    <div className="mx-auto mt-16 w-full max-w-sm">
      <h1 className="text-2xl font-bold">Reset your password</h1>
      <p className="mt-1 text-sm text-zinc-500 dark:text-zinc-400">
        Enter your email and we'll send you a reset link.
      </p>
      <form
        className="mt-6 space-y-4"
        onSubmit={(e) => {
          e.preventDefault()
          forgotPassword.mutate(email)
        }}
      >
        <label className="block">
          <span className="mb-1 block text-sm font-medium">Email</span>
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            autoFocus
            className="w-full rounded-lg border border-zinc-300 bg-white px-3 py-2 outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-zinc-700 dark:bg-zinc-900"
          />
        </label>
        <button
          type="submit"
          disabled={forgotPassword.isPending}
          className="w-full rounded-lg bg-indigo-600 px-4 py-2 font-medium text-white transition hover:bg-indigo-500 disabled:opacity-50"
        >
          {forgotPassword.isPending ? 'Sending…' : 'Send reset link'}
        </button>
      </form>
      <p className="mt-4 text-sm text-zinc-500 dark:text-zinc-400">
        <Link to="/login" className="font-medium text-indigo-600 hover:underline dark:text-indigo-400">
          Back to sign in
        </Link>
      </p>
    </div>
  )
}
