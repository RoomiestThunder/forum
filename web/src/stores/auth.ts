import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface Session {
  userId: number
  username: string
  accessToken: string
  refreshToken: string
}

interface AuthState {
  userId: number | null
  username: string | null
  accessToken: string | null
  refreshToken: string | null
  setSession: (session: Session) => void
  clearSession: () => void
}

export const useAuth = create<AuthState>()(
  persist(
    (set) => ({
      userId: null,
      username: null,
      accessToken: null,
      refreshToken: null,
      setSession: ({ userId, username, accessToken, refreshToken }) =>
        set({ userId, username, accessToken, refreshToken }),
      clearSession: () =>
        set({ userId: null, username: null, accessToken: null, refreshToken: null }),
    }),
    { name: 'forum.auth' },
  ),
)

export function isLoggedIn(): boolean {
  return useAuth.getState().refreshToken !== null
}
