import { lazy, useEffect } from 'react'
import { Link, Route, Routes } from 'react-router'
import Layout from './components/Layout'
import RequireAuth from './components/RequireAuth'
import Feed from './pages/Feed'

// Everything except the landing page is loaded on demand, so a visitor
// downloads the code for the page they open rather than the whole app - most of
// all PostDetail, which carries the markdown renderer.
const loadPostDetail = () => import('./pages/PostDetail')
const PostDetail = lazy(loadPostDetail)
const PostEditor = lazy(() => import('./pages/PostEditor'))
const Login = lazy(() => import('./pages/Login'))
const Register = lazy(() => import('./pages/Register'))
const ForgotPassword = lazy(() => import('./pages/ForgotPassword'))
const ResetPassword = lazy(() => import('./pages/ResetPassword'))
const VerifyEmail = lazy(() => import('./pages/VerifyEmail'))
const OAuthCallback = lazy(() => import('./pages/OAuthCallback'))
const Profile = lazy(() => import('./pages/Profile'))
const Search = lazy(() => import('./pages/Search'))
const Messages = lazy(() => import('./pages/Messages'))
const MessageThread = lazy(() => import('./pages/MessageThread'))
const Settings = lazy(() => import('./pages/Settings'))
const Admin = lazy(() => import('./pages/Admin'))

function NotFound() {
  return (
    <div className="py-16 text-center">
      <h1 className="text-3xl font-bold">404</h1>
      <p className="mt-2 text-zinc-500 dark:text-zinc-400">This page doesn't exist.</p>
      <Link to="/" className="mt-4 inline-block font-medium text-indigo-600 hover:underline dark:text-indigo-400">
        Back to the feed
      </Link>
    </div>
  )
}

function App() {
  // The feed is where almost everyone lands, and opening a post is the next thing
  // they do. Fetch that page's code once the browser is idle, after the feed has
  // rendered, so the click doesn't wait on the largest chunk but the first load
  // doesn't pay for it either.
  useEffect(() => {
    const idle = window.requestIdleCallback ?? ((cb: () => void) => window.setTimeout(cb, 1500))
    const handle = idle(() => void loadPostDetail())
    return () => {
      if (window.cancelIdleCallback) window.cancelIdleCallback(handle)
      else window.clearTimeout(handle)
    }
  }, [])

  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Feed />} />
        <Route path="post/:id" element={<PostDetail />} />
        <Route
          path="post/:id/edit"
          element={
            <RequireAuth>
              <PostEditor />
            </RequireAuth>
          }
        />
        <Route
          path="new"
          element={
            <RequireAuth>
              <PostEditor />
            </RequireAuth>
          }
        />
        <Route path="login" element={<Login />} />
        <Route path="register" element={<Register />} />
        <Route path="forgot-password" element={<ForgotPassword />} />
        <Route path="reset-password" element={<ResetPassword />} />
        <Route path="verify-email" element={<VerifyEmail />} />
        <Route path="oauth/callback" element={<OAuthCallback />} />
        <Route path="u/:username" element={<Profile />} />
        <Route path="search" element={<Search />} />
        <Route
          path="messages"
          element={
            <RequireAuth>
              <Messages />
            </RequireAuth>
          }
        />
        <Route
          path="messages/:username"
          element={
            <RequireAuth>
              <MessageThread />
            </RequireAuth>
          }
        />
        <Route
          path="settings"
          element={
            <RequireAuth>
              <Settings />
            </RequireAuth>
          }
        />
        <Route
          path="admin"
          element={
            <RequireAuth>
              <Admin />
            </RequireAuth>
          }
        />
        <Route path="*" element={<NotFound />} />
      </Route>
    </Routes>
  )
}

export default App
