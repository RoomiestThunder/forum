package database

import (
	"database/sql"
	"log/slog"
	"time"

	"forum/internal/errors"
	"forum/internal/handlers"
)

// Store defines the interface for all database operations.
type Store interface {
	// User operations
	CreateUser(email, username, passwordHash string) (int, error)
	GetUserByEmail(email string) (*handlers.User, error)
	GetUserByID(id int) (*handlers.User, error)
	GetUserByUsername(username string) (*handlers.User, error)

	// Post operations
	CreatePost(userID int, title, content string) (int, error)
	GetPost(id int) (*handlers.Post, error)
	GetPosts(limit, offset int) ([]*handlers.Post, error)
	UpdatePost(id, userID int, title, content string) error
	DeletePost(id, userID int) error
	CountPosts() (int, error)

	// Comment operations
	CreateComment(postID, userID int, content string) (int, error)
	GetCommentsByPostID(postID int) ([]*handlers.Comment, error)
	UpdateComment(id, userID int, content string) error
	DeleteComment(id, userID int) error

	// Session operations
	CreateSession(userID int, uuid string) error
	GetSessionUserID(uuid string) (int, error)
	DeleteSession(uuid string) error
	GetSessionByUserID(userID int) (string, error)

	// Like/Vote operations
	TogglePostLike(postID, userID int, isLike bool) error
	ToggleCommentLike(commentID, userID int, isLike bool) error
	GetPostLikesCount(postID int) (int, int, error)
	GetCommentLikesCount(commentID int) (int, int, error)

	// Utility
	Close() error
	Health() error
}

// SQLiteStore implements Store interface for SQLite.
type SQLiteStore struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewSQLiteStore(db *sql.DB, logger *slog.Logger) Store {
	return &SQLiteStore{
		db:     db,
		logger: logger,
	}
}

func NewSQLiteStoreWithoutLogger(db *sql.DB) Store {
	return &SQLiteStore{
		db:     db,
		logger: nil,
	}
}

func (s *SQLiteStore) CreateUser(email, username, passwordHash string) (int, error) {
	result, err := s.db.Exec(
		"INSERT INTO users (email, username, password) VALUES (?, ?, ?)",
		email, username, passwordHash,
	)
	if err != nil {
		return 0, errors.DatabaseError(err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, errors.DatabaseError(err)
	}
	return int(id), nil
}

// GetUserByEmail retrieves a user by email address.
func (s *SQLiteStore) GetUserByEmail(email string) (*handlers.User, error) {
	var u handlers.User
	err := s.db.QueryRow(
		"SELECT id, email, username, password FROM users WHERE email = ?",
		email,
	).Scan(&u.ID, &u.Email, &u.Username, &u.Password)

	if err == sql.ErrNoRows {
		return nil, errors.NotFoundError("User")
	}
	if err != nil {
		return nil, errors.DatabaseError(err)
	}
	return &u, nil
}

// GetUserByID retrieves a user by ID.
func (s *SQLiteStore) GetUserByID(id int) (*handlers.User, error) {
	var u handlers.User
	err := s.db.QueryRow(
		"SELECT id, email, username, password FROM users WHERE id = ?",
		id,
	).Scan(&u.ID, &u.Email, &u.Username, &u.Password)

	if err == sql.ErrNoRows {
		return nil, errors.NotFoundError("User")
	}
	if err != nil {
		return nil, errors.DatabaseError(err)
	}
	return &u, nil
}

// GetUserByUsername retrieves a user by username.
func (s *SQLiteStore) GetUserByUsername(username string) (*handlers.User, error) {
	var u handlers.User
	err := s.db.QueryRow(
		"SELECT id, email, username, password FROM users WHERE username = ?",
		username,
	).Scan(&u.ID, &u.Email, &u.Username, &u.Password)

	if err == sql.ErrNoRows {
		return nil, errors.NotFoundError("User")
	}
	if err != nil {
		return nil, errors.DatabaseError(err)
	}
	return &u, nil
}

// Post operations

// CreatePost inserts a new post.
func (s *SQLiteStore) CreatePost(userID int, title, content string) (int, error) {
	result, err := s.db.Exec(
		"INSERT INTO posts (user_id, title, content, created_at) VALUES (?, ?, ?, ?)",
		userID, title, content, time.Now(),
	)
	if err != nil {
		return 0, errors.DatabaseError(err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, errors.DatabaseError(err)
	}
	return int(id), nil
}

// GetPost retrieves a post by ID.
func (s *SQLiteStore) GetPost(id int) (*handlers.Post, error) {
	var p handlers.Post
	var createdAt string

	err := s.db.QueryRow(`
		SELECT posts.id, posts.user_id, posts.title, posts.content, posts.created_at, users.username
		FROM posts
		JOIN users ON posts.user_id = users.id
		WHERE posts.id = ?
	`, id).Scan(&p.ID, &p.UserID, &p.Title, &p.Content, &createdAt, &p.Author)

	if err == sql.ErrNoRows {
		return nil, errors.NotFoundError("Post")
	}
	if err != nil {
		return nil, errors.DatabaseError(err)
	}

	p.CreatedAt = createdAt
	return &p, nil
}

// GetPosts retrieves a paginated list of posts.
func (s *SQLiteStore) GetPosts(limit, offset int) ([]*handlers.Post, error) {
	rows, err := s.db.Query(`
		SELECT posts.id, posts.user_id, posts.title, posts.content, posts.created_at, users.username
		FROM posts
		JOIN users ON posts.user_id = users.id
		ORDER BY posts.created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)

	if err != nil {
		return nil, errors.DatabaseError(err)
	}
	defer rows.Close()

	var posts []*handlers.Post
	for rows.Next() {
		var p handlers.Post
		var createdAt string

		if err := rows.Scan(&p.ID, &p.UserID, &p.Title, &p.Content, &createdAt, &p.Author); err != nil {
			return nil, errors.DatabaseError(err)
		}
		p.CreatedAt = createdAt
		posts = append(posts, &p)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.DatabaseError(err)
	}
	return posts, nil
}

// UpdatePost updates an existing post.
func (s *SQLiteStore) UpdatePost(id, userID int, title, content string) error {
	result, err := s.db.Exec(
		"UPDATE posts SET title = ?, content = ? WHERE id = ? AND user_id = ?",
		title, content, id, userID,
	)

	if err != nil {
		return errors.DatabaseError(err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.DatabaseError(err)
	}

	if rowsAffected == 0 {
		return errors.PermissionError("update post")
	}
	return nil
}

// DeletePost deletes a post.
func (s *SQLiteStore) DeletePost(id, userID int) error {
	result, err := s.db.Exec(
		"DELETE FROM posts WHERE id = ? AND user_id = ?",
		id, userID,
	)

	if err != nil {
		return errors.DatabaseError(err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.DatabaseError(err)
	}

	if rowsAffected == 0 {
		return errors.PermissionError("delete post")
	}
	return nil
}

// CountPosts returns the total number of posts.
func (s *SQLiteStore) CountPosts() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM posts").Scan(&count)
	if err != nil {
		return 0, errors.DatabaseError(err)
	}
	return count, nil
}

// Comment operations

// CreateComment inserts a new comment.
func (s *SQLiteStore) CreateComment(postID, userID int, content string) (int, error) {
	result, err := s.db.Exec(
		"INSERT INTO comments (post_id, user_id, content, created_at) VALUES (?, ?, ?, ?)",
		postID, userID, content, time.Now(),
	)
	if err != nil {
		return 0, errors.DatabaseError(err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, errors.DatabaseError(err)
	}
	return int(id), nil
}

// GetCommentsByPostID retrieves all comments for a post.
func (s *SQLiteStore) GetCommentsByPostID(postID int) ([]*handlers.Comment, error) {
	rows, err := s.db.Query(`
		SELECT comments.id, comments.post_id, comments.user_id, comments.content, comments.created_at, users.username
		FROM comments
		JOIN users ON comments.user_id = users.id
		WHERE comments.post_id = ?
		ORDER BY comments.created_at ASC
	`, postID)

	if err != nil {
		return nil, errors.DatabaseError(err)
	}
	defer rows.Close()

	var comments []*handlers.Comment
	for rows.Next() {
		var c handlers.Comment
		var createdAt string

		if err := rows.Scan(&c.ID, &c.PostID, &c.UserID, &c.Content, &createdAt, &c.Author); err != nil {
			return nil, errors.DatabaseError(err)
		}
		c.CreatedAt = createdAt
		comments = append(comments, &c)
	}

	if err = rows.Err(); err != nil {
		return nil, errors.DatabaseError(err)
	}
	return comments, nil
}

// UpdateComment updates a comment.
func (s *SQLiteStore) UpdateComment(id, userID int, content string) error {
	result, err := s.db.Exec(
		"UPDATE comments SET content = ? WHERE id = ? AND user_id = ?",
		content, id, userID,
	)

	if err != nil {
		return errors.DatabaseError(err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.DatabaseError(err)
	}

	if rowsAffected == 0 {
		return errors.PermissionError("update comment")
	}
	return nil
}

// DeleteComment deletes a comment.
func (s *SQLiteStore) DeleteComment(id, userID int) error {
	result, err := s.db.Exec(
		"DELETE FROM comments WHERE id = ? AND user_id = ?",
		id, userID,
	)

	if err != nil {
		return errors.DatabaseError(err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.DatabaseError(err)
	}

	if rowsAffected == 0 {
		return errors.PermissionError("delete comment")
	}
	return nil
}

// Session operations

// CreateSession creates a new session.
func (s *SQLiteStore) CreateSession(userID int, uuid string) error {
	_, err := s.db.Exec(
		"INSERT INTO sessions (user_id, uuid, expires) VALUES (?, ?, ?)",
		userID, uuid, time.Now().Add(24*time.Hour),
	)
	if err != nil {
		return errors.DatabaseError(err)
	}
	return nil
}

// GetSessionUserID retrieves the user ID for a session.
func (s *SQLiteStore) GetSessionUserID(uuid string) (int, error) {
	var userID int
	var expires time.Time

	err := s.db.QueryRow(
		"SELECT user_id, expires FROM sessions WHERE uuid = ?",
		uuid,
	).Scan(&userID, &expires)

	if err == sql.ErrNoRows {
		return 0, errors.NotFoundError("Session")
	}
	if err != nil {
		return 0, errors.DatabaseError(err)
	}

	if time.Now().After(expires) {
		_ = s.DeleteSession(uuid)
		return 0, errors.NotFoundError("Session (expired)")
	}

	return userID, nil
}

// DeleteSession deletes a session.
func (s *SQLiteStore) DeleteSession(uuid string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE uuid = ?", uuid)
	if err != nil {
		return errors.DatabaseError(err)
	}
	return nil
}

// GetSessionByUserID retrieves the session UUID for a user.
func (s *SQLiteStore) GetSessionByUserID(userID int) (string, error) {
	var uuid string
	err := s.db.QueryRow(
		"SELECT uuid FROM sessions WHERE user_id = ? AND expires > ? LIMIT 1",
		userID, time.Now(),
	).Scan(&uuid)

	if err == sql.ErrNoRows {
		return "", errors.NotFoundError("Session")
	}
	if err != nil {
		return "", errors.DatabaseError(err)
	}
	return uuid, nil
}

// Like/Vote operations

// TogglePostLike toggles a like on a post.
func (s *SQLiteStore) TogglePostLike(postID, userID int, isLike bool) error {
	var existingLike int
	err := s.db.QueryRow(
		"SELECT is_like FROM post_likes WHERE post_id = ? AND user_id = ?",
		postID, userID,
	).Scan(&existingLike)

	if err == sql.ErrNoRows {
		// No existing like, create new one
		_, err := s.db.Exec(
			"INSERT INTO post_likes (post_id, user_id, is_like) VALUES (?, ?, ?)",
			postID, userID, isLike,
		)
		return err
	}

	if err != nil {
		return errors.DatabaseError(err)
	}

	// Like exists, update or delete
	if existingLike == (map[bool]int{true: 1, false: 0}[isLike]) {
		// Same vote, delete it
		_, err := s.db.Exec(
			"DELETE FROM post_likes WHERE post_id = ? AND user_id = ?",
			postID, userID,
		)
		return err
	}

	// Different vote, update it
	_, err = s.db.Exec(
		"UPDATE post_likes SET is_like = ? WHERE post_id = ? AND user_id = ?",
		isLike, postID, userID,
	)
	return err
}

// ToggleCommentLike toggles a like on a comment.
func (s *SQLiteStore) ToggleCommentLike(commentID, userID int, isLike bool) error {
	var existingLike int
	err := s.db.QueryRow(
		"SELECT is_like FROM comment_likes WHERE comment_id = ? AND user_id = ?",
		commentID, userID,
	).Scan(&existingLike)

	if err == sql.ErrNoRows {
		_, err := s.db.Exec(
			"INSERT INTO comment_likes (comment_id, user_id, is_like) VALUES (?, ?, ?)",
			commentID, userID, isLike,
		)
		return err
	}

	if err != nil {
		return errors.DatabaseError(err)
	}

	if existingLike == (map[bool]int{true: 1, false: 0}[isLike]) {
		_, err := s.db.Exec(
			"DELETE FROM comment_likes WHERE comment_id = ? AND user_id = ?",
			commentID, userID,
		)
		return err
	}

	_, err = s.db.Exec(
		"UPDATE comment_likes SET is_like = ? WHERE comment_id = ? AND user_id = ?",
		isLike, commentID, userID,
	)
	return err
}

// GetPostLikesCount returns likes and dislikes count for a post.
func (s *SQLiteStore) GetPostLikesCount(postID int) (int, int, error) {
	var likes, dislikes int
	err := s.db.QueryRow(
		"SELECT COUNT(CASE WHEN is_like = 1 THEN 1 END), COUNT(CASE WHEN is_like = 0 THEN 1 END) FROM post_likes WHERE post_id = ?",
		postID,
	).Scan(&likes, &dislikes)

	if err != nil {
		return 0, 0, errors.DatabaseError(err)
	}
	return likes, dislikes, nil
}

// GetCommentLikesCount returns likes and dislikes count for a comment.
func (s *SQLiteStore) GetCommentLikesCount(commentID int) (int, int, error) {
	var likes, dislikes int
	err := s.db.QueryRow(
		"SELECT COUNT(CASE WHEN is_like = 1 THEN 1 END), COUNT(CASE WHEN is_like = 0 THEN 1 END) FROM comment_likes WHERE comment_id = ?",
		commentID,
	).Scan(&likes, &dislikes)

	if err != nil {
		return 0, 0, errors.DatabaseError(err)
	}
	return likes, dislikes, nil
}

// Utility

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Health checks the database connection.
func (s *SQLiteStore) Health() error {
	return s.db.Ping()
}
