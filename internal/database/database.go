package database

import (
	"forum/internal/models"
)

// Store defines the interface for all database operations.
type Store interface {
	// User operations
	CreateUser(email, username, passwordHash string) (int, error)
	GetUserByEmail(email string) (*models.User, error)
	GetUserByID(id int) (*models.User, error)
	GetUserByUsername(username string) (*models.User, error)
	UpdateUserAvatar(userID int, avatarURL string) error
	UpdateUserBio(userID int, bio string) error

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
	CreatePost(userID int, title, content string, categoryIDs []int, imageURL string) (int, error)
	GetPost(id int) (*models.Post, error)
	GetPosts(limit, offset int) ([]*models.Post, error)
	GetPostsByUser(userID, limit, offset int) ([]*models.Post, error)
	GetPostsByCategory(categoryID, limit, offset int) ([]*models.Post, error)
	GetLikedPostsByUser(userID, limit, offset int) ([]*models.Post, error)
	UpdatePost(id, userID int, title, content string) error
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
	CreateComment(postID, userID int, content string) (int, error)
	GetCommentsByPostID(postID int) ([]*models.Comment, error)
	UpdateComment(id, userID int, content string) error
	DeleteComment(id, userID int) error

	// Session operations
	CreateSession(userID int, uuid string) error
	GetSessionUserID(uuid string) (int, error)
	DeleteSession(uuid string) error
	DeleteSessionsByUser(userID int) error

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

	// Utility
	Close() error
	Health() error
}
