export interface Category {
  id: number
  name: string
}

export interface Tag {
  id: number
  name: string
}

export interface Post {
  id: number
  user_id: number
  title: string
  content: string
  created_at: string
  author: string
  likes: number
  dislikes: number
  categories: Category[] | null
  tags: Tag[] | null
  comment_count: number
  image_url?: string
  pinned: boolean
}

export interface Comment {
  id: number
  post_id: number
  user_id: number
  content: string
  created_at: string
  author: string
  likes: number
  dislikes: number
}

export interface AuthResponse {
  access_token: string
  refresh_token: string
  user_id: number
  username: string
}

export interface CurrentUser {
  id: number
  email: string
  username: string
  avatar_url: string
  bio: string
  is_admin: boolean
  is_moderator: boolean
  email_verified: boolean
  created_at: string
}

export interface PublicProfile {
  username: string
  avatar_url: string
  bio: string
  created_at: string
  post_count: number
}

export interface NewCommentPayload {
  post_id: number
  post_title: string
  comment_id: number
  actor: string
}

export interface NewLikePayload {
  post_id: number
  post_title: string
  actor: string
}

export type NotificationPayload = NewCommentPayload | NewLikePayload

export interface Notification {
  id: number
  type: 'new_comment' | 'new_like'
  payload: NotificationPayload
  read: boolean
  created_at: string
}

export interface AdminUser {
  id: number
  email: string
  username: string
  avatar_url: string
  is_admin: boolean
  is_moderator: boolean
  banned: boolean
  created_at: string
}

export interface Report {
  id: number
  reporter_id: number
  reporter_username: string
  target_type: 'post' | 'comment'
  target_id: number
  reason: string
  status: 'open' | 'resolved'
  created_at: string
}

export interface Conversation {
  id: number
  other_username: string
  other_avatar_url: string
  last_message: string
  last_message_at: string
  unread_count: number
}

export interface DirectMessage {
  id: number
  conversation_id: number
  sender_id: number
  sender: string
  content: string
  created_at: string
  read_at?: string
}
