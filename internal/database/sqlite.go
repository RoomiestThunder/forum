package database

import (
	"database/sql"
	"log/slog"
	"time"

	apperrors "forum/internal/errors"
	"forum/internal/models"

	_ "github.com/mattn/go-sqlite3"
)

type SQLiteStore struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewSQLiteStore(db *sql.DB, logger *slog.Logger) Store {
	return &SQLiteStore{db: db, logger: logger}
}

func NewSQLiteStoreWithoutLogger(db *sql.DB) Store {
	return &SQLiteStore{db: db}
}

// InitSQLite opens SQLite, runs schema, returns Store.
// _loc=UTC ensures DATETIME columns are scanned as UTC time.Time values.
func InitSQLite(path string) (*sql.DB, error) {
	dsn := path
	if path != ":memory:" {
		dsn = path + "?_loc=UTC"
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	if err := sqliteSchema(db); err != nil {
		return nil, err
	}
	return db, nil
}

func sqliteSchema(db *sql.DB) error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT UNIQUE NOT NULL,
		username TEXT NOT NULL,
		password TEXT NOT NULL,
		avatar_url TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS categories (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL
	);
	CREATE TABLE IF NOT EXISTS posts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER,
		title TEXT,
		content TEXT,
		image_url TEXT NOT NULL DEFAULT '',
		created_at DATETIME,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);
	CREATE TABLE IF NOT EXISTS post_categories (
		post_id INTEGER,
		category_id INTEGER,
		FOREIGN KEY(post_id) REFERENCES posts(id),
		FOREIGN KEY(category_id) REFERENCES categories(id)
	);
	CREATE TABLE IF NOT EXISTS comments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		post_id INTEGER,
		user_id INTEGER,
		content TEXT,
		created_at DATETIME,
		FOREIGN KEY(post_id) REFERENCES posts(id),
		FOREIGN KEY(user_id) REFERENCES users(id)
	);
	CREATE TABLE IF NOT EXISTS post_likes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		post_id INTEGER,
		user_id INTEGER,
		is_like BOOLEAN,
		FOREIGN KEY(post_id) REFERENCES posts(id),
		FOREIGN KEY(user_id) REFERENCES users(id)
	);
	CREATE TABLE IF NOT EXISTS comment_likes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		comment_id INTEGER,
		user_id INTEGER,
		is_like BOOLEAN,
		FOREIGN KEY(comment_id) REFERENCES comments(id),
		FOREIGN KEY(user_id) REFERENCES users(id)
	);
	CREATE TABLE IF NOT EXISTS sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER,
		uuid TEXT UNIQUE,
		expires DATETIME,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);
	CREATE TABLE IF NOT EXISTS refresh_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		token TEXT UNIQUE NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);
	CREATE TABLE IF NOT EXISTS uploads (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		filename TEXT NOT NULL,
		object_key TEXT NOT NULL,
		content_type TEXT NOT NULL,
		size INTEGER NOT NULL,
		url TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);
	`)
	if err != nil {
		return err
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM categories").Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		_, err = db.Exec("INSERT INTO categories (name) VALUES (?), (?), (?)", "General", "Programming", "Offtopic")
		return err
	}
	return nil
}

// ---- User operations ----

func (s *SQLiteStore) CreateUser(email, username, passwordHash string) (int, error) {
	res, err := s.db.Exec("INSERT INTO users (email, username, password) VALUES (?, ?, ?)", email, username, passwordHash)
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func (s *SQLiteStore) GetUserByEmail(email string) (*models.User, error) {
	var u models.User
	err := s.db.QueryRow("SELECT id, email, username, password, avatar_url FROM users WHERE LOWER(email) = LOWER(?)", email).
		Scan(&u.ID, &u.Email, &u.Username, &u.Password, &u.AvatarURL)
	if err == sql.ErrNoRows {
		return nil, apperrors.NotFoundError("User")
	}
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	return &u, nil
}

func (s *SQLiteStore) GetUserByID(id int) (*models.User, error) {
	var u models.User
	err := s.db.QueryRow("SELECT id, email, username, password, avatar_url FROM users WHERE id = ?", id).
		Scan(&u.ID, &u.Email, &u.Username, &u.Password, &u.AvatarURL)
	if err == sql.ErrNoRows {
		return nil, apperrors.NotFoundError("User")
	}
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	return &u, nil
}

func (s *SQLiteStore) GetUserByUsername(username string) (*models.User, error) {
	var u models.User
	err := s.db.QueryRow("SELECT id, email, username, password, avatar_url FROM users WHERE LOWER(username) = LOWER(?)", username).
		Scan(&u.ID, &u.Email, &u.Username, &u.Password, &u.AvatarURL)
	if err == sql.ErrNoRows {
		return nil, apperrors.NotFoundError("User")
	}
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	return &u, nil
}

func (s *SQLiteStore) UpdateUserAvatar(userID int, avatarURL string) error {
	_, err := s.db.Exec("UPDATE users SET avatar_url = ? WHERE id = ?", avatarURL, userID)
	return err
}

// ---- Post operations ----

func (s *SQLiteStore) CreatePost(userID int, title, content string, categoryIDs []int) (int, error) {
	res, err := s.db.Exec("INSERT INTO posts (user_id, title, content, created_at) VALUES (?, ?, ?, ?)", userID, title, content, time.Now())
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	postID, _ := res.LastInsertId()
	for _, cid := range categoryIDs {
		if _, err := s.db.Exec("INSERT INTO post_categories (post_id, category_id) VALUES (?, ?)", postID, cid); err != nil {
			return 0, apperrors.DatabaseError(err)
		}
	}
	return int(postID), nil
}

// postBaseQuery is the common SELECT for posts with inline aggregated counts.
// This eliminates the N+1 fillPostMeta pattern.
const postBaseQuery = `
SELECT p.id, p.user_id, p.title, p.content, p.created_at, u.username,
       COALESCE(p.image_url,''),
       COALESCE(SUM(CASE WHEN pl.is_like = 1 THEN 1 ELSE 0 END), 0) AS likes,
       COALESCE(SUM(CASE WHEN pl.is_like = 0 THEN 1 ELSE 0 END), 0) AS dislikes,
       COUNT(DISTINCT c.id) AS comment_count
FROM posts p
JOIN users u ON p.user_id = u.id
LEFT JOIN post_likes pl ON p.id = pl.post_id
LEFT JOIN comments c ON p.id = c.post_id`

func (s *SQLiteStore) GetPost(id int) (*models.Post, error) {
	query := postBaseQuery + " WHERE p.id = ? GROUP BY p.id, u.username"
	p, err := s.scanPost(s.db.QueryRow(query, id))
	if err == sql.ErrNoRows {
		return nil, apperrors.NotFoundError("Post")
	}
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	s.loadCategories(p)
	return p, nil
}

func (s *SQLiteStore) GetPosts(limit, offset int) ([]*models.Post, error) {
	return s.queryPosts(postBaseQuery+`
		GROUP BY p.id, u.username ORDER BY p.created_at DESC LIMIT ? OFFSET ?`, limit, offset)
}

func (s *SQLiteStore) GetPostsByUser(userID, limit, offset int) ([]*models.Post, error) {
	return s.queryPosts(postBaseQuery+`
		WHERE p.user_id = ? GROUP BY p.id, u.username ORDER BY p.created_at DESC LIMIT ? OFFSET ?`, userID, limit, offset)
}

func (s *SQLiteStore) GetPostsByCategory(categoryID, limit, offset int) ([]*models.Post, error) {
	q := `
SELECT p.id, p.user_id, p.title, p.content, p.created_at, u.username,
       COALESCE(p.image_url,''),
       COALESCE(SUM(CASE WHEN pl.is_like = 1 THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN pl.is_like = 0 THEN 1 ELSE 0 END), 0),
       COUNT(DISTINCT c.id)
FROM posts p
JOIN users u ON p.user_id = u.id
JOIN post_categories pc ON p.id = pc.post_id
LEFT JOIN post_likes pl ON p.id = pl.post_id
LEFT JOIN comments c ON p.id = c.post_id
WHERE pc.category_id = ?
GROUP BY p.id, u.username ORDER BY p.created_at DESC LIMIT ? OFFSET ?`
	return s.queryPosts(q, categoryID, limit, offset)
}

func (s *SQLiteStore) GetLikedPostsByUser(userID, limit, offset int) ([]*models.Post, error) {
	q := `
SELECT p.id, p.user_id, p.title, p.content, p.created_at, u.username,
       COALESCE(p.image_url,''),
       COALESCE(SUM(CASE WHEN pl2.is_like = 1 THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN pl2.is_like = 0 THEN 1 ELSE 0 END), 0),
       COUNT(DISTINCT c.id)
FROM posts p
JOIN users u ON p.user_id = u.id
JOIN post_likes liked ON p.id = liked.post_id AND liked.user_id = ? AND liked.is_like = 1
LEFT JOIN post_likes pl2 ON p.id = pl2.post_id
LEFT JOIN comments c ON p.id = c.post_id
GROUP BY p.id, u.username ORDER BY p.created_at DESC LIMIT ? OFFSET ?`
	return s.queryPosts(q, userID, limit, offset)
}

func (s *SQLiteStore) scanPost(row *sql.Row) (*models.Post, error) {
	var p models.Post
	err := row.Scan(&p.ID, &p.UserID, &p.Title, &p.Content, &p.CreatedAt, &p.Author,
		&p.ImageURL, &p.Likes, &p.Dislikes, &p.CommentCount)
	return &p, err
}

func (s *SQLiteStore) queryPosts(query string, args ...interface{}) ([]*models.Post, error) {
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

// loadCategories fetches categories for a post in one extra query per post
// (unavoidable without a GROUP_CONCAT approach, but it's only one query now).
func (s *SQLiteStore) loadCategories(p *models.Post) {
	rows, err := s.db.Query(
		"SELECT c.id, c.name FROM categories c JOIN post_categories pc ON c.id = pc.category_id WHERE pc.post_id = ?",
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

func (s *SQLiteStore) UpdatePost(id, userID int, title, content string) error {
	res, err := s.db.Exec("UPDATE posts SET title = ?, content = ? WHERE id = ? AND user_id = ?", title, content, id, userID)
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return apperrors.PermissionError("update post")
	}
	return nil
}

func (s *SQLiteStore) DeletePost(id, userID int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	defer tx.Rollback()

	stmts := []struct{ q string; args []interface{} }{
		{"DELETE FROM post_categories WHERE post_id = ?", []interface{}{id}},
		{"DELETE FROM post_likes WHERE post_id = ?", []interface{}{id}},
		{"DELETE FROM comment_likes WHERE comment_id IN (SELECT id FROM comments WHERE post_id = ?)", []interface{}{id}},
		{"DELETE FROM comments WHERE post_id = ?", []interface{}{id}},
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s.q, s.args...); err != nil {
			return apperrors.DatabaseError(err)
		}
	}

	res, err := tx.Exec("DELETE FROM posts WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return apperrors.PermissionError("delete post")
	}
	return tx.Commit()
}

func (s *SQLiteStore) CountPosts() (int, error) {
	var n int
	return n, s.db.QueryRow("SELECT COUNT(*) FROM posts").Scan(&n)
}

func (s *SQLiteStore) CountPostsByUser(userID int) (int, error) {
	var n int
	return n, s.db.QueryRow("SELECT COUNT(*) FROM posts WHERE user_id = ?", userID).Scan(&n)
}

func (s *SQLiteStore) CountPostsByCategory(categoryID int) (int, error) {
	var n int
	return n, s.db.QueryRow("SELECT COUNT(*) FROM post_categories WHERE category_id = ?", categoryID).Scan(&n)
}

func (s *SQLiteStore) CountLikedPostsByUser(userID int) (int, error) {
	var n int
	return n, s.db.QueryRow("SELECT COUNT(DISTINCT post_id) FROM post_likes WHERE user_id = ? AND is_like = 1", userID).Scan(&n)
}

// SearchPosts - SQLite uses LIKE (PostgreSQL uses tsvector FTS)
func (s *SQLiteStore) SearchPosts(query string, limit, offset int) ([]*models.Post, int, error) {
	like := "%" + query + "%"
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM posts WHERE title LIKE ? OR content LIKE ?", like, like).Scan(&total); err != nil {
		return nil, 0, apperrors.DatabaseError(err)
	}
	posts, err := s.queryPosts(`
		SELECT p.id, p.user_id, p.title, p.content, p.created_at, u.username, COALESCE(p.image_url,'')
		FROM posts p JOIN users u ON p.user_id = u.id
		WHERE p.title LIKE ? OR p.content LIKE ?
		ORDER BY p.created_at DESC LIMIT ? OFFSET ?`, like, like, limit, offset)
	return posts, total, err
}

// ---- Category operations ----

func (s *SQLiteStore) GetCategories() ([]*models.Category, error) {
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

func (s *SQLiteStore) CreateComment(postID, userID int, content string) (int, error) {
	res, err := s.db.Exec("INSERT INTO comments (post_id, user_id, content, created_at) VALUES (?, ?, ?, ?)", postID, userID, content, time.Now())
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func (s *SQLiteStore) GetCommentsByPostID(postID int) ([]*models.Comment, error) {
	rows, err := s.db.Query(`
		SELECT c.id, c.post_id, c.user_id, c.content, c.created_at, u.username
		FROM comments c JOIN users u ON c.user_id = u.id
		WHERE c.post_id = ? ORDER BY c.created_at ASC`, postID)
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	defer rows.Close()
	var comments []*models.Comment
	for rows.Next() {
		var c models.Comment
		_ = rows.Scan(&c.ID, &c.PostID, &c.UserID, &c.Content, &c.CreatedAt, &c.Author)
		s.db.QueryRow("SELECT COUNT(*) FROM comment_likes WHERE comment_id = ? AND is_like = 1", c.ID).Scan(&c.Likes)
		s.db.QueryRow("SELECT COUNT(*) FROM comment_likes WHERE comment_id = ? AND is_like = 0", c.ID).Scan(&c.Dislikes)
		comments = append(comments, &c)
	}
	return comments, nil
}

func (s *SQLiteStore) UpdateComment(id, userID int, content string) error {
	res, err := s.db.Exec("UPDATE comments SET content = ? WHERE id = ? AND user_id = ?", content, id, userID)
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return apperrors.PermissionError("update comment")
	}
	return nil
}

func (s *SQLiteStore) DeleteComment(id, userID int) error {
	_, _ = s.db.Exec("DELETE FROM comment_likes WHERE comment_id = ?", id)
	res, err := s.db.Exec("DELETE FROM comments WHERE id = ? AND user_id = ?", id, userID)
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

func (s *SQLiteStore) CreateSession(userID int, uuid string) error {
	_, err := s.db.Exec("INSERT INTO sessions (user_id, uuid, expires) VALUES (?, ?, ?)", userID, uuid, time.Now().Add(24*time.Hour))
	return err
}

func (s *SQLiteStore) GetSessionUserID(uuid string) (int, error) {
	var userID int
	var expires time.Time
	err := s.db.QueryRow("SELECT user_id, expires FROM sessions WHERE uuid = ?", uuid).Scan(&userID, &expires)
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

func (s *SQLiteStore) DeleteSession(uuid string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE uuid = ?", uuid)
	return err
}

func (s *SQLiteStore) DeleteSessionsByUser(userID int) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE user_id = ?", userID)
	return err
}

// ---- Refresh token operations ----

func (s *SQLiteStore) CreateRefreshToken(userID int, token string) error {
	expires := time.Now().Add(7 * 24 * time.Hour)
	_, err := s.db.Exec("INSERT INTO refresh_tokens (user_id, token, expires_at) VALUES (?, ?, ?)", userID, token, expires)
	return err
}

func (s *SQLiteStore) GetRefreshToken(token string) (*models.RefreshToken, error) {
	var rt models.RefreshToken
	err := s.db.QueryRow("SELECT id, user_id, token, expires_at FROM refresh_tokens WHERE token = ?", token).
		Scan(&rt.ID, &rt.UserID, &rt.Token, &rt.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, apperrors.NotFoundError("RefreshToken")
	}
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	return &rt, nil
}

func (s *SQLiteStore) DeleteRefreshToken(token string) error {
	_, err := s.db.Exec("DELETE FROM refresh_tokens WHERE token = ?", token)
	return err
}

func (s *SQLiteStore) DeleteRefreshTokensByUser(userID int) error {
	_, err := s.db.Exec("DELETE FROM refresh_tokens WHERE user_id = ?", userID)
	return err
}

// ---- Like operations ----

func (s *SQLiteStore) TogglePostLike(postID, userID int, isLike bool) error {
	var existing bool
	err := s.db.QueryRow("SELECT is_like FROM post_likes WHERE post_id = ? AND user_id = ?", postID, userID).Scan(&existing)
	if err == sql.ErrNoRows {
		_, err = s.db.Exec("INSERT INTO post_likes (post_id, user_id, is_like) VALUES (?, ?, ?)", postID, userID, isLike)
		return err
	}
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	if existing == isLike {
		_, err = s.db.Exec("DELETE FROM post_likes WHERE post_id = ? AND user_id = ?", postID, userID)
	} else {
		_, err = s.db.Exec("UPDATE post_likes SET is_like = ? WHERE post_id = ? AND user_id = ?", isLike, postID, userID)
	}
	return err
}

func (s *SQLiteStore) ToggleCommentLike(commentID, userID int, isLike bool) error {
	var existing bool
	err := s.db.QueryRow("SELECT is_like FROM comment_likes WHERE comment_id = ? AND user_id = ?", commentID, userID).Scan(&existing)
	if err == sql.ErrNoRows {
		_, err = s.db.Exec("INSERT INTO comment_likes (comment_id, user_id, is_like) VALUES (?, ?, ?)", commentID, userID, isLike)
		return err
	}
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	if existing == isLike {
		_, err = s.db.Exec("DELETE FROM comment_likes WHERE comment_id = ? AND user_id = ?", commentID, userID)
	} else {
		_, err = s.db.Exec("UPDATE comment_likes SET is_like = ? WHERE comment_id = ? AND user_id = ?", isLike, commentID, userID)
	}
	return err
}

func (s *SQLiteStore) GetPostLikesCount(postID int) (int, int, error) {
	var likes, dislikes int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM post_likes WHERE post_id = ? AND is_like = 1", postID).Scan(&likes)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM post_likes WHERE post_id = ? AND is_like = 0", postID).Scan(&dislikes)
	return likes, dislikes, nil
}

func (s *SQLiteStore) GetCommentLikesCount(commentID int) (int, int, error) {
	var likes, dislikes int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM comment_likes WHERE comment_id = ? AND is_like = 1", commentID).Scan(&likes)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM comment_likes WHERE comment_id = ? AND is_like = 0", commentID).Scan(&dislikes)
	return likes, dislikes, nil
}

// ---- Upload operations ----

func (s *SQLiteStore) CreateUpload(userID int, filename, objectKey, contentType string, size int64, url string) (int, error) {
	res, err := s.db.Exec("INSERT INTO uploads (user_id, filename, object_key, content_type, size, url) VALUES (?, ?, ?, ?, ?, ?)",
		userID, filename, objectKey, contentType, size, url)
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func (s *SQLiteStore) GetUploadsByUser(userID int) ([]*models.Upload, error) {
	rows, err := s.db.Query("SELECT id, user_id, filename, object_key, content_type, size, url, created_at FROM uploads WHERE user_id = ?", userID)
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

func (s *SQLiteStore) Close() error { return s.db.Close() }
func (s *SQLiteStore) Health() error { return s.db.Ping() }
