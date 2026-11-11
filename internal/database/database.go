package database

import (
	"time"

	"forum/internal/models"
)

// Store defines the interface for all database operations.
type Store interface {
	// User operations
	CreateUser(email, username, passwordHash string) (int, error)
	GetUserByEmail(email string) (*models.User, error)
	GetUserByID(id int) (*models.User, error)
	GetUserByUsername(username string) (*models.User, error)
	SearchUsers(prefix string, limit int) ([]*models.UserSummary, error)
	UpdateUserAvatar(userID int, avatarURL string) error
	UpdateUserBio(userID int, bio string) error
	UpdateUserPassword(userID int, passwordHash string) error

	// Account security operations
	SetEmailVerified(userID int) error
	CreateEmailVerificationToken(userID int, token string, expiresAt time.Time) error
	ConsumeEmailVerificationToken(token string) (int, error)
	CreatePasswordResetToken(userID int, token string, expiresAt time.Time) error
	ConsumePasswordResetToken(token string) (int, error)
	FindOrCreateOAuthUser(provider, providerUserID, email, preferredUsername, avatarURL string) (*models.User, error)

	// Admin operations
	ListUsers(limit, offset int) ([]*models.User, error)
	CountUsers() (int, error)
	SetUserRole(userID int, isAdmin, isModerator bool) error
	SetUserBanned(userID int, banned bool) error
	SetPostPinned(postID int, pinned bool) error
	AdminDeletePost(id int) error
	AdminDeleteComment(id int) error
	CreateReport(reporterID int, targetType string, targetID int, reason string) (int, error)
	ListReports(status string, limit, offset int) ([]*models.Report, error)
	CountOpenReports() (int, error)
	ResolveReport(id int) error

	// Post operations
	CreatePost(userID int, title, content string, categoryIDs []int, imageURL string, draft bool) (int, error)
	GetPost(id int) (*models.Post, error)
	GetPosts(limit, offset int) ([]*models.Post, error)
	GetPostsByUser(userID, limit, offset int) ([]*models.Post, error)
	GetDraftPostsByUser(userID, limit, offset int) ([]*models.Post, error)
	GetPostsByCategory(categoryID, limit, offset int) ([]*models.Post, error)
	GetLikedPostsByUser(userID, limit, offset int) ([]*models.Post, error)
	UpdatePost(id, userID int, title, content string) error
	SetPostDraft(id, userID int, draft bool) error
	DeletePost(id, userID int) error
	CountPosts() (int, error)
	CountPostsByUser(userID int) (int, error)
	CountPostsByCategory(categoryID int) (int, error)
	CountLikedPostsByUser(userID int) (int, error)
	SearchPosts(query string, limit, offset int) ([]*models.Post, int, error)

	// Category operations
	GetCategories() ([]*models.Category, error)

	// Tag operations
	SetPostTags(postID int, tagNames []string) error
	GetPostsByTag(tagName string, limit, offset int) ([]*models.Post, error)
	CountPostsByTag(tagName string) (int, error)
	SearchTags(prefix string, limit int) ([]*models.Tag, error)

	// Bookmark operations
	ToggleBookmark(postID, userID int, bookmarked bool) error
	IsBookmarked(postID, userID int) (bool, error)
	GetBookmarkedPostsByUser(userID, limit, offset int) ([]*models.Post, error)
	CountBookmarkedPostsByUser(userID int) (int, error)

	// Comment operations
	// CreateComment adds a comment; a non-nil parentID makes it a reply, and
	// the parent must belong to the same post.
	CreateComment(postID, userID int, parentID *int, content string) (int, error)
	GetComment(id int) (*models.Comment, error)
	GetCommentsByPostID(postID int) ([]*models.Comment, error)
	UpdateComment(id, userID int, content string) error
	// DeleteComment removes a comment, or tombstones it if it has replies.
	// Deleting a comment you don't own is a permission error.
	DeleteComment(id, userID int) error

	// Refresh token operations
	CreateRefreshToken(userID int, token string) error
	GetRefreshToken(token string) (*models.RefreshToken, error)
	DeleteRefreshToken(token string) error
	DeleteRefreshTokensByUser(userID int) error

	// Like/Vote operations
	TogglePostLike(postID, userID int, isLike bool) error
	ToggleCommentLike(commentID, userID int, isLike bool) error
	GetPostLikesCount(postID int) (int, int, error)
	GetCommentLikesCount(commentID int) (int, int, error)

	// Upload operations
	CreateUpload(userID int, filename, objectKey, contentType string, size int64, url string) (int, error)
	GetUploadsByUser(userID int) ([]*models.Upload, error)

	// Notification operations
	CreateNotification(userID int, notifType, payload string) (int, error)
	GetNotificationsByUser(userID, limit, offset int) ([]*models.NotificationRecord, error)
	CountUnreadNotifications(userID int) (int, error)
	MarkNotificationRead(id, userID int) error
	MarkAllNotificationsRead(userID int) error

	// Direct message operations
	GetOrCreateConversation(userA, userB int) (int, error)
	ListConversations(userID int) ([]*models.Conversation, error)
	GetMessages(conversationID, limit, offset int) ([]*models.Message, error)
	CreateMessage(conversationID, senderID int, content string) (*models.Message, error)
	MarkConversationRead(conversationID, userID int) error
	CountUnreadMessages(userID int) (int, error)

	// Audit log operations
	CreateAuditEntry(actorID int, action, targetType string, targetID int, details string) error
	ListAuditEntries(limit, offset int) ([]*models.AuditEntry, error)
	CountAuditEntries() (int, error)

	// Block operations
	BlockUser(blockerID, blockedID int) error
	UnblockUser(blockerID, blockedID int) error
	IsBlocked(blockerID, blockedID int) (bool, error)
	ListBlockedUsers(blockerID int) ([]*models.UserSummary, error)

	// Utility
	Close() error
	Health() error
}
