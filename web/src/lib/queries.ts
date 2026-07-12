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
  CurrentUser,
  Notification,
  Post,
  PublicProfile,
  Report,
  Tag,
} from './types'
import { useAuth } from '../stores/auth'

export const PAGE_SIZE = 10

export interface FeedParams {
  filter?: 'myposts' | 'liked' | 'saved'
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
    }) => api<{ id: number }>('/posts', { method: 'POST', body: JSON.stringify(body) }),
    onSuccess: () => invalidate(),
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
    mutationFn: (content: string) =>
      api<{ id: number }>(`/posts/${postId}`, {
        method: 'POST',
        body: JSON.stringify({ content }),
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
