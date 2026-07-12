import { useState } from 'react'
import { Loader2, Shield, ShieldCheck, ShieldOff } from 'lucide-react'
import { Link, Navigate } from 'react-router'
import {
  useAdminReports,
  useAdminUsers,
  useMe,
  useResolveReport,
  useSetUserBanned,
  useSetUserRole,
} from '../lib/queries'
import { timeAgo } from '../lib/time'
import type { AdminUser } from '../lib/types'

function UsersTab() {
  const [page, setPage] = useState(1)
  const { data, isLoading } = useAdminUsers(page)
  const setRole = useSetUserRole()
  const setBanned = useSetUserBanned()
  const { data: me } = useMe()

  if (isLoading) {
    return (
      <div className="flex justify-center py-12">
        <Loader2 className="animate-spin text-zinc-400" />
      </div>
    )
  }

  const roleLabel = (u: AdminUser) => (u.is_admin ? 'Admin' : u.is_moderator ? 'Moderator' : 'Member')

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-zinc-200 text-left text-zinc-500 dark:border-zinc-800 dark:text-zinc-400">
            <th className="py-2 pr-4 font-medium">User</th>
            <th className="py-2 pr-4 font-medium">Role</th>
            <th className="py-2 pr-4 font-medium">Joined</th>
            <th className="py-2 pr-4 font-medium">Status</th>
            <th className="py-2 font-medium">Actions</th>
          </tr>
        </thead>
        <tbody>
          {(data?.users ?? []).map((u) => {
            const isSelf = u.id === me?.id
            return (
              <tr key={u.id} className="border-b border-zinc-100 dark:border-zinc-800/60">
                <td className="py-2 pr-4">
                  <Link to={`/u/${u.username}`} className="font-medium hover:underline">
                    {u.username}
                  </Link>
                  <div className="text-xs text-zinc-400">{u.email}</div>
                </td>
                <td className="py-2 pr-4">{roleLabel(u)}</td>
                <td className="py-2 pr-4 text-zinc-500 dark:text-zinc-400">{timeAgo(u.created_at)}</td>
                <td className="py-2 pr-4">
                  {u.banned ? (
                    <span className="rounded-full bg-red-100 px-2 py-0.5 text-xs font-medium text-red-700 dark:bg-red-950/50 dark:text-red-300">
                      Banned
                    </span>
                  ) : (
                    <span className="rounded-full bg-emerald-100 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-300">
                      Active
                    </span>
                  )}
                </td>
                <td className="py-2">
                  <div className="flex flex-wrap gap-1.5">
                    {!u.is_admin && (
                      <button
                        type="button"
                        onClick={() =>
                          setRole.mutate({ userId: u.id, isAdmin: false, isModerator: !u.is_moderator })
                        }
                        title={u.is_moderator ? 'Remove moderator' : 'Make moderator'}
                        className="rounded-lg border border-zinc-300 p-1.5 text-zinc-600 transition hover:bg-zinc-100 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
                      >
                        {u.is_moderator ? <ShieldOff size={14} /> : <Shield size={14} />}
                      </button>
                    )}
                    <button
                      type="button"
                      onClick={() => setRole.mutate({ userId: u.id, isAdmin: !u.is_admin, isModerator: false })}
                      disabled={isSelf}
                      title={u.is_admin ? 'Remove admin' : 'Make admin'}
                      className="rounded-lg border border-zinc-300 p-1.5 text-zinc-600 transition hover:bg-zinc-100 disabled:opacity-40 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
                    >
                      <ShieldCheck size={14} />
                    </button>
                    <button
                      type="button"
                      onClick={() => setBanned.mutate({ userId: u.id, banned: !u.banned })}
                      disabled={isSelf}
                      className="rounded-lg border border-zinc-300 px-2 py-1 text-xs font-medium text-zinc-600 transition hover:bg-zinc-100 disabled:opacity-40 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
                    >
                      {u.banned ? 'Unban' : 'Ban'}
                    </button>
                  </div>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>

      {data && data.total > data.users.length * page && (
        <div className="mt-3 flex justify-center">
          <button
            type="button"
            onClick={() => setPage((p) => p + 1)}
            className="rounded-lg border border-zinc-300 px-3 py-1.5 text-sm font-medium text-zinc-600 transition hover:bg-zinc-100 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
          >
            Load more
          </button>
        </div>
      )}
    </div>
  )
}

function ReportsTab() {
  const [status, setStatus] = useState<'open' | 'resolved'>('open')
  const { data, isLoading } = useAdminReports(status)
  const resolve = useResolveReport()

  return (
    <div>
      <div className="mb-3 flex gap-2">
        {(['open', 'resolved'] as const).map((s) => (
          <button
            key={s}
            type="button"
            onClick={() => setStatus(s)}
            className={`rounded-full px-3 py-1 text-sm font-medium capitalize transition ${
              status === s
                ? 'bg-indigo-600 text-white'
                : 'bg-zinc-100 text-zinc-600 hover:bg-zinc-200 dark:bg-zinc-800 dark:text-zinc-300 dark:hover:bg-zinc-700'
            }`}
          >
            {s}
          </button>
        ))}
      </div>

      {isLoading && (
        <div className="flex justify-center py-12">
          <Loader2 className="animate-spin text-zinc-400" />
        </div>
      )}

      {!isLoading && (data ?? []).length === 0 && (
        <p className="py-8 text-center text-sm text-zinc-500 dark:text-zinc-400">No {status} reports.</p>
      )}

      <div className="space-y-2">
        {(data ?? []).map((r) => (
          <div
            key={r.id}
            className="rounded-lg border border-zinc-200 bg-white p-3 text-sm dark:border-zinc-800 dark:bg-zinc-900"
          >
            <div className="flex items-center justify-between">
              <span className="font-medium">
                {r.target_type === 'post' ? 'Post' : 'Comment'} #{r.target_id}
              </span>
              <span className="text-xs text-zinc-400">{timeAgo(r.created_at)}</span>
            </div>
            <p className="mt-1 text-zinc-600 dark:text-zinc-400">{r.reason}</p>
            <div className="mt-2 flex items-center justify-between">
              <span className="text-xs text-zinc-400">Reported by {r.reporter_username}</span>
              <div className="flex items-center gap-2">
                <Link
                  to={r.target_type === 'post' ? `/post/${r.target_id}` : `/post/${r.target_id}#comment-${r.target_id}`}
                  className="text-xs font-medium text-indigo-600 hover:underline dark:text-indigo-400"
                >
                  View
                </Link>
                {status === 'open' && (
                  <button
                    type="button"
                    onClick={() => resolve.mutate(r.id)}
                    disabled={resolve.isPending}
                    className="rounded-lg bg-indigo-600 px-2.5 py-1 text-xs font-medium text-white transition hover:bg-indigo-500 disabled:opacity-50"
                  >
                    Resolve
                  </button>
                )}
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

export default function Admin() {
  const { data: me, isLoading } = useMe()
  const [tab, setTab] = useState<'reports' | 'users'>('reports')

  if (isLoading) {
    return (
      <div className="flex justify-center py-16">
        <Loader2 className="animate-spin text-zinc-400" />
      </div>
    )
  }

  if (!me || (!me.is_admin && !me.is_moderator)) {
    return <Navigate to="/" replace />
  }

  return (
    <div>
      <h1 className="text-2xl font-bold">Admin</h1>
      <div className="mt-4 mb-4 flex gap-2 border-b border-zinc-200 dark:border-zinc-800">
        <button
          type="button"
          onClick={() => setTab('reports')}
          className={`border-b-2 px-1 pb-2 text-sm font-medium transition ${
            tab === 'reports'
              ? 'border-indigo-600 text-indigo-600 dark:text-indigo-400'
              : 'border-transparent text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100'
          }`}
        >
          Reports
        </button>
        {me.is_admin && (
          <button
            type="button"
            onClick={() => setTab('users')}
            className={`border-b-2 px-1 pb-2 text-sm font-medium transition ${
              tab === 'users'
                ? 'border-indigo-600 text-indigo-600 dark:text-indigo-400'
                : 'border-transparent text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100'
            }`}
          >
            Users
          </button>
        )}
      </div>

      {tab === 'reports' && <ReportsTab />}
      {tab === 'users' && me.is_admin && <UsersTab />}
    </div>
  )
}
