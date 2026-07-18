import { Link, Route, Routes } from 'react-router'
import Layout from './components/Layout'
import RequireAuth from './components/RequireAuth'
import Admin from './pages/Admin'
import Feed from './pages/Feed'
import ForgotPassword from './pages/ForgotPassword'
import Login from './pages/Login'
import MessageThread from './pages/MessageThread'
import Messages from './pages/Messages'
import OAuthCallback from './pages/OAuthCallback'
import PostDetail from './pages/PostDetail'
import PostEditor from './pages/PostEditor'
import Profile from './pages/Profile'
import Register from './pages/Register'
import ResetPassword from './pages/ResetPassword'
import Search from './pages/Search'
import Settings from './pages/Settings'
import VerifyEmail from './pages/VerifyEmail'

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
