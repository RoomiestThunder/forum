import { useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router'
import { useResetPassword } from '../lib/queries'
import { PASSWORD_MAX_BYTES, PASSWORD_MIN_LENGTH } from '../lib/validation'

export default function ResetPassword() {
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') ?? ''
  const navigate = useNavigate()
  const [password, setPassword] = useState('')
  const resetPassword = useResetPassword()

  if (!token) {
    return (
      <div className="mx-auto mt-16 w-full max-w-sm text-center">
        <h1 className="text-2xl font-bold">Invalid link</h1>
        <p className="mt-2 text-sm text-zinc-500 dark:text-zinc-400">
          This password reset link is missing its token.
        </p>
        <Link to="/forgot-password" className="mt-4 inline-block text-sm font-medium text-indigo-600 hover:underline dark:text-indigo-400">
          Request a new one
        </Link>
      </div>
    )
  }

  if (resetPassword.isSuccess) {
    return (
      <div className="mx-auto mt-16 w-full max-w-sm text-center">
        <h1 className="text-2xl font-bold">Password updated</h1>
        <p className="mt-2 text-sm text-zinc-500 dark:text-zinc-400">
          You can now sign in with your new password.
        </p>
        <button
          type="button"
          onClick={() => navigate('/login')}
          className="mt-4 rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-500"
        >
          Sign in
        </button>
      </div>
    )
  }

  return (
    <div className="mx-auto mt-16 w-full max-w-sm">
      <h1 className="text-2xl font-bold">Choose a new password</h1>
      <form
        className="mt-6 space-y-4"
        onSubmit={(e) => {
          e.preventDefault()
          resetPassword.mutate({ token, new_password: password })
        }}
      >
        {resetPassword.isError && (
          <p className="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950/50 dark:text-red-300">
            {resetPassword.error.message}
          </p>
        )}
        <label className="block">
          <span className="mb-1 block text-sm font-medium">New password</span>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            minLength={PASSWORD_MIN_LENGTH}
            maxLength={PASSWORD_MAX_BYTES}
            autoFocus
            autoComplete="new-password"
            className="w-full rounded-lg border border-zinc-300 bg-white px-3 py-2 outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-zinc-700 dark:bg-zinc-900"
          />
        </label>
        <button
          type="submit"
          disabled={resetPassword.isPending}
          className="w-full rounded-lg bg-indigo-600 px-4 py-2 font-medium text-white transition hover:bg-indigo-500 disabled:opacity-50"
        >
          {resetPassword.isPending ? 'Saving…' : 'Reset password'}
        </button>
      </form>
    </div>
  )
}
