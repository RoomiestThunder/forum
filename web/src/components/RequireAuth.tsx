import type { ReactNode } from 'react'
import { Navigate, useLocation } from 'react-router'
import { useAuth } from '../stores/auth'

export default function RequireAuth({ children }: { children: ReactNode }) {
  const loggedIn = useAuth((s) => s.refreshToken !== null)
  const location = useLocation()

  if (!loggedIn) {
    return <Navigate to="/login" state={{ from: location.pathname }} replace />
  }
  return children
}
