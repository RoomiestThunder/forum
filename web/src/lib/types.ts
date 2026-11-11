export interface Category {
  id: number
  name: string
}

export interface UserSummary {
  id: number
  username: string
  avatar_url: string
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
  draft: boolean
}

export interface Comment {
  id: number
  post_id: number
  /** 0 for a deleted comment: the author isn't revealed. */
  user_id: number
  /** The comment this one replies to, or null for a top-level comment. */
  parent_id: number | null
  /** Nesting level: 0 for top-level, one more than the parent for a reply. */
  depth: number
  content: string
  created_at: string
  author: string
  likes: number
  dislikes: number
  /** A tombstone: deleted, but kept because other comments reply to it. */
  deleted: boolean
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

export interface NewReplyPayload {
  post_id: number
  post_title: string
  comment_id: number
  parent_id: number
  actor: string
}

export interface NewLikePayload {
  post_id: number
  post_title: string
  actor: string
}

export interface NewMentionPayload {
  post_id: number
  post_title: string
  comment_id: number
  actor: string
}

export type NotificationPayload =
  | NewCommentPayload
  | NewReplyPayload
  | NewLikePayload
  | NewMentionPayload

export interface Notification {
  id: number
  type: 'new_comment' | 'new_reply' | 'new_like' | 'new_mention'
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

export interface AuditEntry {
  id: number
  actor_id: number
  actor: string
  /** e.g. "post.delete", "user.ban" - see describeAudit for the full set. */
  action: string
  target_type: string
  target_id: number
  /** A snapshot of what was acted on; its keys depend on the action. */
  details: Record<string, unknown>
  created_at: string
}
