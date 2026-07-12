import { useState } from 'react'
import { Link, Outlet, useNavigate } from 'react-router'
import { Moon, Sun, PenSquare, LogOut, MessagesSquare } from 'lucide-react'
import { logout } from '../lib/api'
import { useAuth } from '../stores/auth'

function ThemeToggle() {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains('dark'))

  const toggle = () => {
    const next = !dark
    document.documentElement.classList.toggle('dark', next)
    localStorage.setItem('forum.theme', next ? 'dark' : 'light')
    setDark(next)
  }

  return (
    <button
      type="button"
      onClick={toggle}
      aria-label={dark ? 'Switch to light theme' : 'Switch to dark theme'}
      className="rounded-lg p-2 text-zinc-500 transition hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
    >
      {dark ? <Sun size={18} /> : <Moon size={18} />}
    </button>
  )
}

export default function Layout() {
  const navigate = useNavigate()
  const username = useAuth((s) => s.username)

  const handleLogout = async () => {
    await logout()
    navigate('/')
  }

  return (
    <div className="flex min-h-screen flex-col">
      <header className="sticky top-0 z-10 border-b border-zinc-200 bg-white/80 backdrop-blur dark:border-zinc-800 dark:bg-zinc-950/80">
        <div className="mx-auto flex h-14 max-w-4xl items-center gap-3 px-4">
          <Link to="/" className="flex items-center gap-2 font-bold">
            <MessagesSquare size={20} className="text-indigo-600 dark:text-indigo-400" />
            Forum
          </Link>
          <div className="flex-1" />
          <ThemeToggle />
          {username ? (
            <>
              <Link
                to="/new"
                className="flex items-center gap-1.5 rounded-lg bg-indigo-600 px-3 py-1.5 text-sm font-medium text-white transition hover:bg-indigo-500"
              >
                <PenSquare size={15} />
                New post
              </Link>
              <span className="text-sm font-medium">{username}</span>
              <button
                type="button"
                onClick={handleLogout}
                aria-label="Sign out"
                className="rounded-lg p-2 text-zinc-500 transition hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
              >
                <LogOut size={18} />
              </button>
            </>
          ) : (
            <>
              <Link
                to="/login"
                className="rounded-lg px-3 py-1.5 text-sm font-medium text-zinc-600 transition hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800"
              >
                Sign in
              </Link>
              <Link
                to="/register"
                className="rounded-lg bg-indigo-600 px-3 py-1.5 text-sm font-medium text-white transition hover:bg-indigo-500"
              >
                Register
              </Link>
            </>
          )}
        </div>
      </header>
      <main className="mx-auto w-full max-w-4xl flex-1 px-4 py-6">
        <Outlet />
      </main>
      <footer className="border-t border-zinc-200 py-4 text-center text-sm text-zinc-400 dark:border-zinc-800 dark:text-zinc-600">
        Forum — built with Go &amp; React
      </footer>
    </div>
  )
}
