import { useEffect, useRef, useState } from 'react'
import { Loader2 } from 'lucide-react'
import { useNavigate } from 'react-router'
import { useMe, useUpdateBio, useUploadAvatar } from '../lib/queries'
import { useAuth } from '../stores/auth'

export default function Settings() {
  const navigate = useNavigate()
  const username = useAuth((s) => s.username)
  const { data: me, isLoading } = useMe()
  const updateBio = useUpdateBio()
  const uploadAvatar = useUploadAvatar()
  const fileInput = useRef<HTMLInputElement>(null)

  const [bio, setBio] = useState('')

  useEffect(() => {
    if (me) setBio(me.bio)
  }, [me])

  if (isLoading || !me) {
    return (
      <div className="flex justify-center py-16">
        <Loader2 className="animate-spin text-zinc-400" />
      </div>
    )
  }

  const avatarUrl = uploadAvatar.data?.avatar_url ?? me.avatar_url

  return (
    <div className="mx-auto max-w-lg">
      <h1 className="text-2xl font-bold">Profile settings</h1>

      <div className="mt-6 flex items-center gap-4">
        {avatarUrl ? (
          <img src={avatarUrl} alt={username ?? ''} className="h-16 w-16 rounded-full object-cover" />
        ) : (
          <div className="flex h-16 w-16 items-center justify-center rounded-full bg-indigo-100 text-xl font-semibold text-indigo-600 dark:bg-indigo-950 dark:text-indigo-300">
            {username?.slice(0, 1).toUpperCase()}
          </div>
        )}
        <div>
          <input
            ref={fileInput}
            type="file"
            accept="image/png,image/jpeg,image/gif,image/webp"
            className="hidden"
            onChange={(e) => {
              const file = e.target.files?.[0]
              if (file) uploadAvatar.mutate(file)
            }}
          />
          <button
            type="button"
            onClick={() => fileInput.current?.click()}
            disabled={uploadAvatar.isPending}
            className="rounded-lg border border-zinc-300 px-3 py-1.5 text-sm font-medium text-zinc-600 transition hover:bg-zinc-100 disabled:opacity-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
          >
            {uploadAvatar.isPending ? 'Uploading…' : 'Change avatar'}
          </button>
          {uploadAvatar.isError && (
            <p className="mt-1 text-xs text-red-600 dark:text-red-400">{uploadAvatar.error.message}</p>
          )}
        </div>
      </div>

      <form
        className="mt-6"
        onSubmit={(e) => {
          e.preventDefault()
          updateBio.mutate(bio.trim())
        }}
      >
        <label className="block">
          <span className="mb-1 block text-sm font-medium">Bio</span>
          <textarea
            value={bio}
            onChange={(e) => setBio(e.target.value)}
            maxLength={280}
            rows={4}
            className="w-full rounded-lg border border-zinc-300 bg-white px-3 py-2 text-sm outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-zinc-700 dark:bg-zinc-900"
          />
        </label>
        <div className="mt-1 flex items-center justify-between">
          <span className="text-xs text-zinc-400">{bio.length}/280</span>
          {updateBio.isSuccess && <span className="text-xs text-emerald-600 dark:text-emerald-400">Saved</span>}
        </div>
        {updateBio.isError && (
          <p className="mt-1 text-sm text-red-600 dark:text-red-400">{updateBio.error.message}</p>
        )}
        <div className="mt-3 flex gap-2">
          <button
            type="submit"
            disabled={updateBio.isPending}
            className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-indigo-500 disabled:opacity-50"
          >
            Save bio
          </button>
          <button
            type="button"
            onClick={() => navigate(`/u/${username}`)}
            className="rounded-lg px-4 py-2 text-sm font-medium text-zinc-600 transition hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800"
          >
            Back to profile
          </button>
        </div>
      </form>
    </div>
  )
}
