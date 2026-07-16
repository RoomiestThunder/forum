import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import './index.css'
import App from './App.tsx'
import { useAuth } from './stores/auth.ts'

// Apply the saved theme before the first paint to avoid a light-mode flash.
const savedTheme = localStorage.getItem('forum.theme')
if (
  savedTheme === 'dark' ||
  (savedTheme === null && window.matchMedia('(prefers-color-scheme: dark)').matches)
) {
  document.documentElement.classList.add('dark')
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
    },
  },
})

// Every cached query (profile, posts, notifications, ...) is scoped to
// whoever was logged in when it was fetched. Without this, switching
// accounts in the same tab would keep showing the previous user's data
// until each query happened to refetch on its own.
let currentUserId = useAuth.getState().userId
useAuth.subscribe((state) => {
  if (state.userId !== currentUserId) {
    currentUserId = state.userId
    queryClient.clear()
  }
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)
