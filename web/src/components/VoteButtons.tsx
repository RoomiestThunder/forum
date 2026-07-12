import { ThumbsDown, ThumbsUp } from 'lucide-react'
import { useNavigate } from 'react-router'
import { useAuth } from '../stores/auth'

interface Props {
  likes: number
  dislikes: number
  onVote: (isLike: boolean) => void
  disabled?: boolean
}

export default function VoteButtons({ likes, dislikes, onVote, disabled }: Props) {
  const navigate = useNavigate()
  const loggedIn = useAuth((s) => s.refreshToken !== null)

  const vote = (isLike: boolean) => {
    if (!loggedIn) {
      navigate('/login', { state: { from: window.location.pathname } })
      return
    }
    onVote(isLike)
  }

  const buttonClass =
    'flex items-center gap-1.5 rounded-full px-2.5 py-1 text-sm text-zinc-500 transition hover:bg-zinc-100 disabled:opacity-50 dark:text-zinc-400 dark:hover:bg-zinc-800'

  return (
    <div className="flex items-center gap-1">
      <button
        type="button"
        onClick={() => vote(true)}
        disabled={disabled}
        aria-label="Like"
        className={`${buttonClass} hover:text-emerald-600 dark:hover:text-emerald-400`}
      >
        <ThumbsUp size={15} />
        {likes}
      </button>
      <button
        type="button"
        onClick={() => vote(false)}
        disabled={disabled}
        aria-label="Dislike"
        className={`${buttonClass} hover:text-red-600 dark:hover:text-red-400`}
      >
        <ThumbsDown size={15} />
        {dislikes}
      </button>
    </div>
  )
}
