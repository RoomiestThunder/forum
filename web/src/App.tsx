import { Route, Routes } from 'react-router'
import Layout from './components/Layout'
import Login from './pages/Login'
import Register from './pages/Register'

function Placeholder() {
  return (
    <div className="py-16 text-center text-zinc-500 dark:text-zinc-400">
      Feed coming soon.
    </div>
  )
}

function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Placeholder />} />
        <Route path="login" element={<Login />} />
        <Route path="register" element={<Register />} />
      </Route>
    </Routes>
  )
}

export default App
