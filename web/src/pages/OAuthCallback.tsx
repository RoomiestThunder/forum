import { useEffect, useState } from 'react'
import { Loader2, XCircle } from 'lucide-react'
import { Link, useNavigate } from 'react-router'
import { useAuth } from '../stores/auth'

export default function OAuthCallback() {
  const navigate = useNavigate()
  const setSession = useAuth((s) => s.setSession)
  const [error, setError] = useState(false)

  useEffect(() => {
    const params = new URLSearchParams(window.location.hash.slice(1))
    const accessToken = params.get('access_token')
    const refreshToken = params.get('refresh_token')
    const userId = params.get('user_id')
    const username = params.get('username')

    if (!accessToken || !refreshToken || !userId || !username) {
      setError(true)
      return
    }

    setSession({ userId: Number(userId), username, accessToken, refreshToken })
    navigate('/', { replace: true })
  }, [navigate, setSession])

  if (error) {
    return (
      <div className="mx-auto mt-16 w-full max-w-sm text-center">
        <XCircle className="mx-auto mb-2 text-red-500" size={32} />
        <h1 className="text-2xl font-bold">Sign-in failed</h1>
        <p className="mt-2 text-sm text-zinc-500 dark:text-zinc-400">Something went wrong completing sign-in.</p>
        <Link to="/login" className="mt-4 inline-block text-sm font-medium text-indigo-600 hover:underline dark:text-indigo-400">
          Try again
        </Link>
      </div>
    )
  }

  return (
    <div className="flex justify-center py-16">
      <Loader2 className="animate-spin text-zinc-400" />
    </div>
  )
}
