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
