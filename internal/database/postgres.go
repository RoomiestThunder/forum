package database

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"
	"time"

	apperrors "forum/internal/errors"
	"forum/internal/models"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed schema_postgres.sql
var postgresSchema string

type PostgresStore struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewPostgresStore(db *sql.DB, logger *slog.Logger) Store {
	return &PostgresStore{db: db, logger: logger}
}

// InitPostgres opens a PostgreSQL connection pool.
func InitPostgres(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return db, nil
}

// RunPostgresSchema applies the embedded schema to a freshly created database.
// Used in tests and for first-run initialization without a migration tool.
func RunPostgresSchema(db *sql.DB) error {
	_, err := db.Exec(postgresSchema)
	return err
}

// ---- User operations ----

func (s *PostgresStore) CreateUser(email, username, passwordHash string) (int, error) {
	var id int
	err := s.db.QueryRow(
		"INSERT INTO users (email, username, password) VALUES ($1, $2, $3) RETURNING id",
		email, username, passwordHash,
	).Scan(&id)
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	return id, nil
}

func (s *PostgresStore) GetUserByEmail(email string) (*models.User, error) {
	var u models.User
	err := s.db.QueryRow(
		"SELECT id, email, username, password, COALESCE(avatar_url,'') FROM users WHERE LOWER(email) = LOWER($1)", email,
	).Scan(&u.ID, &u.Email, &u.Username, &u.Password, &u.AvatarURL)
	if err == sql.ErrNoRows {
		return nil, apperrors.NotFoundError("User")
	}
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	return &u, nil
}

func (s *PostgresStore) GetUserByID(id int) (*models.User, error) {
	var u models.User
	err := s.db.QueryRow(
		"SELECT id, email, username, password, COALESCE(avatar_url,'') FROM users WHERE id = $1", id,
	).Scan(&u.ID, &u.Email, &u.Username, &u.Password, &u.AvatarURL)
	if err == sql.ErrNoRows {
		return nil, apperrors.NotFoundError("User")
	}
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	return &u, nil
}

func (s *PostgresStore) GetUserByUsername(username string) (*models.User, error) {
	var u models.User
	err := s.db.QueryRow(
		"SELECT id, email, username, password, COALESCE(avatar_url,'') FROM users WHERE LOWER(username) = LOWER($1)", username,
	).Scan(&u.ID, &u.Email, &u.Username, &u.Password, &u.AvatarURL)
	if err == sql.ErrNoRows {
		return nil, apperrors.NotFoundError("User")
	}
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	return &u, nil
}

func (s *PostgresStore) UpdateUserAvatar(userID int, avatarURL string) error {
	_, err := s.db.Exec("UPDATE users SET avatar_url = $1 WHERE id = $2", avatarURL, userID)
	return err
}

// ---- Post operations ----

func (s *PostgresStore) CreatePost(userID int, title, content string, categoryIDs []int) (int, error) {
	var postID int
	err := s.db.QueryRow(
		"INSERT INTO posts (user_id, title, content, created_at) VALUES ($1, $2, $3, $4) RETURNING id",
		userID, title, content, time.Now(),
	).Scan(&postID)
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	for _, cid := range categoryIDs {
		if _, err := s.db.Exec("INSERT INTO post_categories (post_id, category_id) VALUES ($1, $2)", postID, cid); err != nil {
			return 0, apperrors.DatabaseError(err)
		}
	}
	return postID, nil
}

const pgPostBase = `
SELECT p.id, p.user_id, p.title, p.content, p.created_at, u.username,
       COALESCE(p.image_url,''),
       COALESCE(SUM(CASE WHEN pl.is_like = TRUE  THEN 1 ELSE 0 END), 0) AS likes,
       COALESCE(SUM(CASE WHEN pl.is_like = FALSE THEN 1 ELSE 0 END), 0) AS dislikes,
       COUNT(DISTINCT c.id) AS comment_count
FROM posts p
JOIN users u ON p.user_id = u.id
LEFT JOIN post_likes pl ON p.id = pl.post_id
LEFT JOIN comments c ON p.id = c.post_id`

func (s *PostgresStore) GetPost(id int) (*models.Post, error) {
	query := pgPostBase + " WHERE p.id = $1 GROUP BY p.id, u.username"
	var p models.Post
	err := s.db.QueryRow(query, id).
		Scan(&p.ID, &p.UserID, &p.Title, &p.Content, &p.CreatedAt, &p.Author,
			&p.ImageURL, &p.Likes, &p.Dislikes, &p.CommentCount)
	if err == sql.ErrNoRows {
		return nil, apperrors.NotFoundError("Post")
	}
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	s.loadCategories(&p)
	return &p, nil
}

func (s *PostgresStore) GetPosts(limit, offset int) ([]*models.Post, error) {
	return s.queryPosts(pgPostBase+`
		GROUP BY p.id, u.username ORDER BY p.created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
}

func (s *PostgresStore) GetPostsByUser(userID, limit, offset int) ([]*models.Post, error) {
	return s.queryPosts(pgPostBase+`
		WHERE p.user_id = $1 GROUP BY p.id, u.username ORDER BY p.created_at DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset)
}

func (s *PostgresStore) GetPostsByCategory(categoryID, limit, offset int) ([]*models.Post, error) {
	q := `
SELECT p.id, p.user_id, p.title, p.content, p.created_at, u.username,
       COALESCE(p.image_url,''),
       COALESCE(SUM(CASE WHEN pl.is_like = TRUE  THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN pl.is_like = FALSE THEN 1 ELSE 0 END), 0),
       COUNT(DISTINCT c.id)
FROM posts p
JOIN users u ON p.user_id = u.id
JOIN post_categories pc ON p.id = pc.post_id
LEFT JOIN post_likes pl ON p.id = pl.post_id
LEFT JOIN comments c ON p.id = c.post_id
WHERE pc.category_id = $1
GROUP BY p.id, u.username ORDER BY p.created_at DESC LIMIT $2 OFFSET $3`
	return s.queryPosts(q, categoryID, limit, offset)
}

func (s *PostgresStore) GetLikedPostsByUser(userID, limit, offset int) ([]*models.Post, error) {
	q := `
SELECT p.id, p.user_id, p.title, p.content, p.created_at, u.username,
       COALESCE(p.image_url,''),
       COALESCE(SUM(CASE WHEN pl2.is_like = TRUE  THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN pl2.is_like = FALSE THEN 1 ELSE 0 END), 0),
       COUNT(DISTINCT c.id)
FROM posts p
JOIN users u ON p.user_id = u.id
JOIN post_likes liked ON p.id = liked.post_id AND liked.user_id = $1 AND liked.is_like = TRUE
LEFT JOIN post_likes pl2 ON p.id = pl2.post_id
LEFT JOIN comments c ON p.id = c.post_id
GROUP BY p.id, u.username ORDER BY p.created_at DESC LIMIT $2 OFFSET $3`
	return s.queryPosts(q, userID, limit, offset)
}

// SearchPosts uses PostgreSQL full-text search with tsvector.
func (s *PostgresStore) SearchPosts(query string, limit, offset int) ([]*models.Post, int, error) {
	var total int
	if err := s.db.QueryRow(
		"SELECT COUNT(*) FROM posts WHERE search_vector @@ plainto_tsquery('english', $1)", query,
	).Scan(&total); err != nil {
		return nil, 0, apperrors.DatabaseError(err)
	}
	q := pgPostBase + `
		WHERE p.search_vector @@ plainto_tsquery('english', $1)
		GROUP BY p.id, u.username
		ORDER BY ts_rank(p.search_vector, plainto_tsquery('english', $1)) DESC
		LIMIT $2 OFFSET $3`
	posts, err := s.queryPosts(q, query, limit, offset)
	return posts, total, err
}

func (s *PostgresStore) queryPosts(query string, args ...interface{}) ([]*models.Post, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	defer rows.Close()

	var posts []*models.Post
	for rows.Next() {
		var p models.Post
		if err := rows.Scan(&p.ID, &p.UserID, &p.Title, &p.Content, &p.CreatedAt, &p.Author,
			&p.ImageURL, &p.Likes, &p.Dislikes, &p.CommentCount); err != nil {
			return nil, apperrors.DatabaseError(err)
		}
		s.loadCategories(&p)
		posts = append(posts, &p)
	}
	return posts, rows.Err()
}

func (s *PostgresStore) loadCategories(p *models.Post) {
	rows, err := s.db.Query(
		"SELECT c.id, c.name FROM categories c JOIN post_categories pc ON c.id = pc.category_id WHERE pc.post_id = $1",
		p.ID,
	)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var c models.Category
		_ = rows.Scan(&c.ID, &c.Name)
		p.Categories = append(p.Categories, c)
	}
}

func (s *PostgresStore) UpdatePost(id, userID int, title, content string) error {
	res, err := s.db.Exec("UPDATE posts SET title = $1, content = $2 WHERE id = $3 AND user_id = $4", title, content, id, userID)
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return apperrors.PermissionError("update post")
	}
	return nil
}

func (s *PostgresStore) DeletePost(id, userID int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	defer tx.Rollback()

	stmts := []struct {
		q    string
		args []interface{}
	}{
		{"DELETE FROM post_categories WHERE post_id = $1", []interface{}{id}},
		{"DELETE FROM post_likes WHERE post_id = $1", []interface{}{id}},
		{"DELETE FROM comment_likes WHERE comment_id IN (SELECT id FROM comments WHERE post_id = $1)", []interface{}{id}},
		{"DELETE FROM comments WHERE post_id = $1", []interface{}{id}},
	}
	for _, st := range stmts {
		if _, err := tx.Exec(st.q, st.args...); err != nil {
			return apperrors.DatabaseError(err)
		}
	}

	res, err := tx.Exec("DELETE FROM posts WHERE id = $1 AND user_id = $2", id, userID)
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return apperrors.PermissionError("delete post")
	}
	return tx.Commit()
}

func (s *PostgresStore) CountPosts() (int, error) {
	var n int
	return n, s.db.QueryRow("SELECT COUNT(*) FROM posts").Scan(&n)
}

func (s *PostgresStore) CountPostsByUser(userID int) (int, error) {
	var n int
	return n, s.db.QueryRow("SELECT COUNT(*) FROM posts WHERE user_id = $1", userID).Scan(&n)
}

func (s *PostgresStore) CountPostsByCategory(categoryID int) (int, error) {
	var n int
	return n, s.db.QueryRow("SELECT COUNT(*) FROM post_categories WHERE category_id = $1", categoryID).Scan(&n)
}

func (s *PostgresStore) CountLikedPostsByUser(userID int) (int, error) {
	var n int
	return n, s.db.QueryRow("SELECT COUNT(DISTINCT post_id) FROM post_likes WHERE user_id = $1 AND is_like = TRUE", userID).Scan(&n)
}

// ---- Category operations ----

func (s *PostgresStore) GetCategories() ([]*models.Category, error) {
	rows, err := s.db.Query("SELECT id, name FROM categories")
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	defer rows.Close()
	var cats []*models.Category
	for rows.Next() {
		var c models.Category
		_ = rows.Scan(&c.ID, &c.Name)
		cats = append(cats, &c)
	}
	return cats, nil
}

// ---- Comment operations ----

func (s *PostgresStore) CreateComment(postID, userID int, content string) (int, error) {
	var id int
	err := s.db.QueryRow(
		"INSERT INTO comments (post_id, user_id, content, created_at) VALUES ($1, $2, $3, $4) RETURNING id",
		postID, userID, content, time.Now(),
	).Scan(&id)
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	return id, nil
}

func (s *PostgresStore) GetCommentsByPostID(postID int) ([]*models.Comment, error) {
	rows, err := s.db.Query(`
		SELECT c.id, c.post_id, c.user_id, c.content, c.created_at, u.username
		FROM comments c JOIN users u ON c.user_id = u.id
		WHERE c.post_id = $1 ORDER BY c.created_at ASC`, postID)
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	defer rows.Close()
	var comments []*models.Comment
	for rows.Next() {
		var c models.Comment
		_ = rows.Scan(&c.ID, &c.PostID, &c.UserID, &c.Content, &c.CreatedAt, &c.Author)
		_ = s.db.QueryRow("SELECT COUNT(*) FROM comment_likes WHERE comment_id = $1 AND is_like = TRUE", c.ID).Scan(&c.Likes)
		_ = s.db.QueryRow("SELECT COUNT(*) FROM comment_likes WHERE comment_id = $1 AND is_like = FALSE", c.ID).Scan(&c.Dislikes)
		comments = append(comments, &c)
	}
	return comments, nil
}

func (s *PostgresStore) UpdateComment(id, userID int, content string) error {
	res, err := s.db.Exec("UPDATE comments SET content = $1 WHERE id = $2 AND user_id = $3", content, id, userID)
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return apperrors.PermissionError("update comment")
	}
	return nil
}

func (s *PostgresStore) DeleteComment(id, userID int) error {
	_, _ = s.db.Exec("DELETE FROM comment_likes WHERE comment_id = $1", id)
	res, err := s.db.Exec("DELETE FROM comments WHERE id = $1 AND user_id = $2", id, userID)
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return apperrors.PermissionError("delete comment")
	}
	return nil
}

// ---- Session operations ----

func (s *PostgresStore) CreateSession(userID int, uuid string) error {
	_, err := s.db.Exec(
		"INSERT INTO sessions (user_id, uuid, expires) VALUES ($1, $2, $3)",
		userID, uuid, time.Now().Add(24*time.Hour),
	)
	return err
}

func (s *PostgresStore) GetSessionUserID(uuid string) (int, error) {
	var userID int
	var expires time.Time
	err := s.db.QueryRow("SELECT user_id, expires FROM sessions WHERE uuid = $1", uuid).Scan(&userID, &expires)
	if err == sql.ErrNoRows {
		return 0, apperrors.NotFoundError("Session")
	}
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	if time.Now().After(expires) {
		_ = s.DeleteSession(uuid)
		return 0, apperrors.NotFoundError("Session (expired)")
	}
	return userID, nil
}

func (s *PostgresStore) DeleteSession(uuid string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE uuid = $1", uuid)
	return err
}

func (s *PostgresStore) DeleteSessionsByUser(userID int) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE user_id = $1", userID)
	return err
}

// ---- Refresh token operations ----

func (s *PostgresStore) CreateRefreshToken(userID int, token string) error {
	expires := time.Now().Add(7 * 24 * time.Hour)
	_, err := s.db.Exec(
		"INSERT INTO refresh_tokens (user_id, token, expires_at) VALUES ($1, $2, $3)",
		userID, token, expires,
	)
	return err
}

func (s *PostgresStore) GetRefreshToken(token string) (*models.RefreshToken, error) {
	var rt models.RefreshToken
	err := s.db.QueryRow("SELECT id, user_id, token, expires_at FROM refresh_tokens WHERE token = $1", token).
		Scan(&rt.ID, &rt.UserID, &rt.Token, &rt.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, apperrors.NotFoundError("RefreshToken")
	}
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	return &rt, nil
}

func (s *PostgresStore) DeleteRefreshToken(token string) error {
	_, err := s.db.Exec("DELETE FROM refresh_tokens WHERE token = $1", token)
	return err
}

func (s *PostgresStore) DeleteRefreshTokensByUser(userID int) error {
	_, err := s.db.Exec("DELETE FROM refresh_tokens WHERE user_id = $1", userID)
	return err
}

// ---- Like operations ----

func (s *PostgresStore) TogglePostLike(postID, userID int, isLike bool) error {
	var existing bool
	err := s.db.QueryRow("SELECT is_like FROM post_likes WHERE post_id = $1 AND user_id = $2", postID, userID).Scan(&existing)
	if err == sql.ErrNoRows {
		_, err = s.db.Exec("INSERT INTO post_likes (post_id, user_id, is_like) VALUES ($1, $2, $3)", postID, userID, isLike)
		return err
	}
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	if existing == isLike {
		_, err = s.db.Exec("DELETE FROM post_likes WHERE post_id = $1 AND user_id = $2", postID, userID)
	} else {
		_, err = s.db.Exec("UPDATE post_likes SET is_like = $1 WHERE post_id = $2 AND user_id = $3", isLike, postID, userID)
	}
	return err
}

func (s *PostgresStore) ToggleCommentLike(commentID, userID int, isLike bool) error {
	var existing bool
	err := s.db.QueryRow("SELECT is_like FROM comment_likes WHERE comment_id = $1 AND user_id = $2", commentID, userID).Scan(&existing)
	if err == sql.ErrNoRows {
		_, err = s.db.Exec("INSERT INTO comment_likes (comment_id, user_id, is_like) VALUES ($1, $2, $3)", commentID, userID, isLike)
		return err
	}
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	if existing == isLike {
		_, err = s.db.Exec("DELETE FROM comment_likes WHERE comment_id = $1 AND user_id = $2", commentID, userID)
	} else {
		_, err = s.db.Exec("UPDATE comment_likes SET is_like = $1 WHERE comment_id = $2 AND user_id = $3", isLike, commentID, userID)
	}
	return err
}

func (s *PostgresStore) GetPostLikesCount(postID int) (int, int, error) {
	var likes, dislikes int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM post_likes WHERE post_id = $1 AND is_like = TRUE", postID).Scan(&likes)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM post_likes WHERE post_id = $1 AND is_like = FALSE", postID).Scan(&dislikes)
	return likes, dislikes, nil
}

func (s *PostgresStore) GetCommentLikesCount(commentID int) (int, int, error) {
	var likes, dislikes int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM comment_likes WHERE comment_id = $1 AND is_like = TRUE", commentID).Scan(&likes)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM comment_likes WHERE comment_id = $1 AND is_like = FALSE", commentID).Scan(&dislikes)
	return likes, dislikes, nil
}

// ---- Upload operations ----

func (s *PostgresStore) CreateUpload(userID int, filename, objectKey, contentType string, size int64, url string) (int, error) {
	var id int
	err := s.db.QueryRow(
		"INSERT INTO uploads (user_id, filename, object_key, content_type, size, url) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id",
		userID, filename, objectKey, contentType, size, url,
	).Scan(&id)
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	return id, nil
}

func (s *PostgresStore) GetUploadsByUser(userID int) ([]*models.Upload, error) {
	rows, err := s.db.Query("SELECT id, user_id, filename, object_key, content_type, size, url, created_at FROM uploads WHERE user_id = $1", userID)
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	defer rows.Close()
	var uploads []*models.Upload
	for rows.Next() {
		var u models.Upload
		_ = rows.Scan(&u.ID, &u.UserID, &u.Filename, &u.ObjectKey, &u.ContentType, &u.Size, &u.URL, &u.CreatedAt)
		uploads = append(uploads, &u)
	}
	return uploads, nil
}

// ---- Utility ----

func (s *PostgresStore) Close() error { return s.db.Close() }
func (s *PostgresStore) Health() error { return s.db.Ping() }
