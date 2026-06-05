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

	// Post operations
	CreatePost(userID int, title, content string, categoryIDs []int) (int, error)
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

	// Utility
	Close() error
	Health() error
}
