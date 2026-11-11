import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { api, uploadFile } from './api'
import type {
  AdminUser,
  Category,
  Comment,
  Conversation,
  CurrentUser,
  DirectMessage,
  Notification,
  Post,
  PublicProfile,
  Report,
  Tag,
  UserSummary,
} from './types'
import { useAuth } from '../stores/auth'

export const PAGE_SIZE = 10

export interface FeedParams {
  filter?: 'myposts' | 'liked' | 'saved' | 'drafts'
  category?: number
  author?: string
  tag?: string
}

export function usePostsFeed(params: FeedParams) {
  return useInfiniteQuery({
    queryKey: ['posts', params],
    queryFn: async ({ pageParam }) => {
      const search = new URLSearchParams({ page: String(pageParam) })
      if (params.filter) search.set('filter', params.filter)
      if (params.category) search.set('category', String(params.category))
      if (params.author) search.set('author', params.author)
      if (params.tag) search.set('tag', params.tag)
      // The API returns null instead of [] for an empty page.
      return (await api<Post[] | null>(`/posts?${search}`)) ?? []
    },
    initialPageParam: 1,
    getNextPageParam: (lastPage, _all, lastPageParam) =>
      lastPage.length === PAGE_SIZE ? lastPageParam + 1 : undefined,
  })
}

export function usePost(id: number) {
  return useQuery({
    queryKey: ['post', id],
    queryFn: () =>
      api<{ post: Post; comments: Comment[] | null; is_bookmarked?: boolean }>(`/posts/${id}`),
  })
}

export function useTagSuggestions(query: string) {
  return useQuery({
    queryKey: ['tags', query],
    queryFn: () => api<Tag[] | null>(`/tags?q=${encodeURIComponent(query)}`),
    enabled: query.length > 0,
    staleTime: 30_000,
  })
}

export function useSearchUsers(query: string) {
  return useQuery({
    queryKey: ['users', 'search', query],
    queryFn: () => api<UserSummary[] | null>(`/users/search?q=${encodeURIComponent(query)}`),
    enabled: query.length > 0,
    staleTime: 30_000,
  })
}

export function useSearch(query: string, page: number) {
  return useQuery({
    queryKey: ['search', query, page],
    queryFn: () =>
      api<{ posts: Post[] | null; total: number; page: number }>(
        `/search?q=${encodeURIComponent(query)}&page=${page}`,
      ),
    enabled: query.trim().length > 0,
    placeholderData: (prev) => prev,
  })
}

export function useCategories() {
  return useQuery({
    queryKey: ['categories'],
    queryFn: () => api<Category[]>('/categories'),
    staleTime: 5 * 60_000,
  })
}

/** Invalidate every posts-related cache after a write. */
function useInvalidatePosts() {
  const qc = useQueryClient()
  return (postId?: number) => {
    void qc.invalidateQueries({ queryKey: ['posts'] })
    if (postId !== undefined) {
      void qc.invalidateQueries({ queryKey: ['post', postId] })
    }
  }
}

export function useCreatePost() {
  const invalidate = useInvalidatePosts()
  return useMutation({
    mutationFn: (body: {
      title: string
      content: string
      category_ids: number[]
      image_url?: string
      tags?: string[]
      draft?: boolean
    }) => api<{ id: number }>('/posts', { method: 'POST', body: JSON.stringify(body) }),
    onSuccess: () => invalidate(),
  })
}

export function usePublishPost() {
  const invalidate = useInvalidatePosts()
  return useMutation({
    mutationFn: (postId: number) => api(`/posts/${postId}/publish`, { method: 'POST' }),
    onSuccess: (_data, postId) => invalidate(postId),
  })
}

export function useUpdatePost(id: number) {
  const invalidate = useInvalidatePosts()
  return useMutation({
    mutationFn: (body: { title: string; content: string }) =>
      api(`/posts/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
    onSuccess: () => invalidate(id),
  })
}

export function useDeletePost() {
  const invalidate = useInvalidatePosts()
  return useMutation({
    mutationFn: (id: number) => api(`/posts/${id}`, { method: 'DELETE' }),
    onSuccess: () => invalidate(),
  })
}

export function useVotePost() {
  const invalidate = useInvalidatePosts()
  return useMutation({
    mutationFn: ({ postId, isLike }: { postId: number; isLike: boolean }) =>
      api('/like/post', {
        method: 'POST',
        body: JSON.stringify({ post_id: postId, is_like: isLike }),
      }),
    onSuccess: (_data, { postId }) => invalidate(postId),
  })
}

export function useVoteComment(postId: number) {
  const invalidate = useInvalidatePosts()
  return useMutation({
    mutationFn: ({ commentId, isLike }: { commentId: number; isLike: boolean }) =>
      api('/like/comment', {
        method: 'POST',
        body: JSON.stringify({ comment_id: commentId, is_like: isLike }),
      }),
    onSuccess: () => invalidate(postId),
  })
}

export function useCreateComment(postId: number) {
  const invalidate = useInvalidatePosts()
  return useMutation({
    mutationFn: ({ content, parentId }: { content: string; parentId?: number }) =>
      api<{ id: number }>(`/posts/${postId}`, {
        method: 'POST',
        body: JSON.stringify({ content, parent_id: parentId }),
      }),
    onSuccess: () => invalidate(postId),
  })
}

export function useUpdateComment(postId: number) {
  const invalidate = useInvalidatePosts()
  return useMutation({
    mutationFn: ({ commentId, content }: { commentId: number; content: string }) =>
      api(`/comments/${commentId}`, {
        method: 'PUT',
        body: JSON.stringify({ content }),
      }),
    onSuccess: () => invalidate(postId),
  })
}

export function useDeleteComment(postId: number) {
  const invalidate = useInvalidatePosts()
  return useMutation({
    mutationFn: (commentId: number) =>
      api(`/comments/${commentId}`, { method: 'DELETE' }),
    onSuccess: () => invalidate(postId),
  })
}

export function useMe() {
  const loggedIn = useAuth((s) => s.refreshToken !== null)
  return useQuery({
    queryKey: ['me'],
    queryFn: () => api<CurrentUser>('/me'),
    enabled: loggedIn,
  })
}

export function useProfile(username: string) {
  return useQuery({
    queryKey: ['profile', username],
    queryFn: () => api<PublicProfile>(`/users/${username}`),
    enabled: username !== '',
  })
}

export function useUpdateBio() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (bio: string) => api('/me/bio', { method: 'PUT', body: JSON.stringify({ bio }) }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['profile'] })
      void qc.invalidateQueries({ queryKey: ['me'] })
    },
  })
}

export function useUploadAvatar() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (file: File) => uploadFile<{ avatar_url: string }>('/me/avatar', file),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['profile'] })
      void qc.invalidateQueries({ queryKey: ['me'] })
    },
  })
}

export function useUploadPostImage() {
  return useMutation({
    mutationFn: (file: File) => uploadFile<{ id: number; url: string }>('/upload', file),
  })
}

export function useNotifications() {
  const loggedIn = useAuth((s) => s.refreshToken !== null)
  return useQuery({
    queryKey: ['notifications'],
    queryFn: () => api<Notification[] | null>('/notifications'),
    enabled: loggedIn,
  })
}

export function useUnreadNotificationCount() {
  const loggedIn = useAuth((s) => s.refreshToken !== null)
  return useQuery({
    queryKey: ['notifications', 'unread-count'],
    queryFn: () => api<{ count: number }>('/notifications/unread-count'),
    enabled: loggedIn,
  })
}

function useInvalidateNotifications() {
  const qc = useQueryClient()
  return () => {
    void qc.invalidateQueries({ queryKey: ['notifications'] })
  }
}

export function useMarkNotificationRead() {
  const invalidate = useInvalidateNotifications()
  return useMutation({
    mutationFn: (id: number) => api(`/notifications/${id}/read`, { method: 'POST' }),
    onSuccess: () => invalidate(),
  })
}

export function useMarkAllNotificationsRead() {
  const invalidate = useInvalidateNotifications()
  return useMutation({
    mutationFn: () => api('/notifications/read-all', { method: 'POST' }),
    onSuccess: () => invalidate(),
  })
}

export function useToggleBookmark(postId: number) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (bookmarked: boolean) =>
      api('/bookmarks', {
        method: 'POST',
        body: JSON.stringify({ post_id: postId, bookmarked }),
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['post', postId] })
      void qc.invalidateQueries({ queryKey: ['posts'] })
    },
  })
}

export function useCreateReport() {
  return useMutation({
    mutationFn: (body: { target_type: 'post' | 'comment'; target_id: number; reason: string }) =>
      api('/reports', { method: 'POST', body: JSON.stringify(body) }),
  })
}

export function useAdminUsers(page: number) {
  return useQuery({
    queryKey: ['admin', 'users', page],
    queryFn: () => api<{ users: AdminUser[]; total: number }>(`/admin/users?page=${page}`),
  })
}

export function useSetUserRole() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ userId, isAdmin, isModerator }: { userId: number; isAdmin: boolean; isModerator: boolean }) =>
      api(`/admin/users/${userId}/role`, {
        method: 'POST',
        body: JSON.stringify({ is_admin: isAdmin, is_moderator: isModerator }),
      }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['admin', 'users'] }),
  })
}

export function useSetUserBanned() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ userId, banned }: { userId: number; banned: boolean }) =>
      api(`/admin/users/${userId}/ban`, { method: 'POST', body: JSON.stringify({ banned }) }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['admin', 'users'] }),
  })
}

export function useAdminReports(status: 'open' | 'resolved') {
  return useQuery({
    queryKey: ['admin', 'reports', status],
    queryFn: () => api<Report[] | null>(`/admin/reports?status=${status}`),
  })
}

export function useResolveReport() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api(`/admin/reports/${id}/resolve`, { method: 'POST' }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['admin', 'reports'] }),
  })
}

export function useSetPostPinned() {
  const invalidate = useInvalidatePosts()
  return useMutation({
    mutationFn: ({ postId, pinned }: { postId: number; pinned: boolean }) =>
      api(`/admin/posts/${postId}/pin`, { method: 'POST', body: JSON.stringify({ pinned }) }),
    onSuccess: (_data, { postId }) => invalidate(postId),
  })
}

export function useAdminDeletePost() {
  const invalidate = useInvalidatePosts()
  return useMutation({
    mutationFn: (postId: number) => api(`/admin/posts/${postId}`, { method: 'DELETE' }),
    onSuccess: () => invalidate(),
  })
}

export function useAdminDeleteComment(postId: number) {
  const invalidate = useInvalidatePosts()
  return useMutation({
    mutationFn: (commentId: number) => api(`/admin/comments/${commentId}`, { method: 'DELETE' }),
    onSuccess: () => invalidate(postId),
  })
}

export function useConversations() {
  const loggedIn = useAuth((s) => s.refreshToken !== null)
  return useQuery({
    queryKey: ['conversations'],
    queryFn: () => api<Conversation[] | null>('/messages'),
    enabled: loggedIn,
  })
}

export function useUnreadMessageCount() {
  const loggedIn = useAuth((s) => s.refreshToken !== null)
  return useQuery({
    queryKey: ['messages', 'unread-count'],
    queryFn: () => api<{ count: number }>('/messages/unread-count'),
    enabled: loggedIn,
  })
}

export function useThread(username: string) {
  return useQuery({
    queryKey: ['messages', username],
    queryFn: () => api<{ messages: DirectMessage[] | null }>(`/messages/${username}`),
    enabled: username !== '',
  })
}

export function useSendMessage(username: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (content: string) =>
      api<DirectMessage>(`/messages/${username}`, { method: 'POST', body: JSON.stringify({ content }) }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['messages', username] })
      void qc.invalidateQueries({ queryKey: ['conversations'] })
    },
  })
}

export function useBlockedUsers() {
  const loggedIn = useAuth((s) => s.refreshToken !== null)
  return useQuery({
    queryKey: ['blocks'],
    queryFn: () => api<UserSummary[]>('/blocks'),
    enabled: loggedIn,
  })
}

/** Whether `username` is on the signed-in user's block list (names are case-insensitive). */
export function useIsBlocked(username: string): boolean {
  const { data: blocked } = useBlockedUsers()
  const wanted = username.toLowerCase()
  return blocked?.some((u) => u.username.toLowerCase() === wanted) ?? false
}

export function useSetBlocked() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ username, blocked }: { username: string; blocked: boolean }) =>
      api(`/blocks/${encodeURIComponent(username)}`, { method: blocked ? 'PUT' : 'DELETE' }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['blocks'] }),
  })
}

export function useAuthProviders() {
  return useQuery({
    queryKey: ['auth-providers'],
    queryFn: () => api<{ github: boolean }>('/auth/providers'),
    staleTime: Infinity,
  })
}

export function useForgotPassword() {
  return useMutation({
    mutationFn: (email: string) => api('/auth/forgot-password', { method: 'POST', body: JSON.stringify({ email }) }),
  })
}

export function useResetPassword() {
  return useMutation({
    mutationFn: (body: { token: string; new_password: string }) =>
      api('/auth/reset-password', { method: 'POST', body: JSON.stringify(body) }),
  })
}

export function useVerifyEmail() {
  return useMutation({
    mutationFn: (token: string) => api('/auth/verify-email', { method: 'POST', body: JSON.stringify({ token }) }),
  })
}

export function useResendVerification() {
  return useMutation({
    mutationFn: () => api('/auth/resend-verification', { method: 'POST' }),
  })
}
