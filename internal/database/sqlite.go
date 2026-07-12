package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
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
	if path == ":memory:" {
		// Each pool connection would otherwise get its own empty in-memory
		// database, so everything must go through a single connection.
		db.SetMaxOpenConns(1)
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
		avatar_url TEXT NOT NULL DEFAULT '',
		bio TEXT NOT NULL DEFAULT '',
		is_admin BOOLEAN NOT NULL DEFAULT 0,
		is_moderator BOOLEAN NOT NULL DEFAULT 0,
		banned BOOLEAN NOT NULL DEFAULT 0,
		email_verified BOOLEAN NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
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
		pinned BOOLEAN NOT NULL DEFAULT 0,
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
	CREATE TABLE IF NOT EXISTS tags (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL
	);
	CREATE TABLE IF NOT EXISTS post_tags (
		post_id INTEGER,
		tag_id INTEGER,
		FOREIGN KEY(post_id) REFERENCES posts(id),
		FOREIGN KEY(tag_id) REFERENCES tags(id)
	);
	CREATE TABLE IF NOT EXISTS bookmarks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		post_id INTEGER NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id),
		FOREIGN KEY(post_id) REFERENCES posts(id),
		UNIQUE(user_id, post_id)
	);
	CREATE TABLE IF NOT EXISTS notifications (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		type TEXT NOT NULL,
		payload TEXT NOT NULL,
		read BOOLEAN NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);
	CREATE TABLE IF NOT EXISTS reports (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		reporter_id INTEGER NOT NULL,
		target_type TEXT NOT NULL CHECK (target_type IN ('post', 'comment')),
		target_id   INTEGER NOT NULL,
		reason      TEXT NOT NULL,
		status      TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
		created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(reporter_id) REFERENCES users(id)
	);
	CREATE TABLE IF NOT EXISTS email_verification_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		token TEXT UNIQUE NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);
	CREATE TABLE IF NOT EXISTS password_reset_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		token TEXT UNIQUE NOT NULL,
		expires_at DATETIME NOT NULL,
		used BOOLEAN NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);
	CREATE TABLE IF NOT EXISTS oauth_identities (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		provider TEXT NOT NULL,
		provider_user_id TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id),
		UNIQUE(provider, provider_user_id)
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
	// The very first account on a fresh install becomes an admin - there's
	// no other bootstrap mechanism (no seed script, no CLI), so without this
	// nobody could ever reach the admin panel.
	var userCount int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount); err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	res, err := s.db.Exec(
		"INSERT INTO users (email, username, password, is_admin) VALUES (?, ?, ?, ?)",
		email, username, passwordHash, userCount == 0,
	)
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

const userColumns = "id, email, username, password, avatar_url, bio, is_admin, is_moderator, banned, email_verified, created_at"

func scanUser(row *sql.Row) (*models.User, error) {
	var u models.User
	err := row.Scan(&u.ID, &u.Email, &u.Username, &u.Password, &u.AvatarURL, &u.Bio,
		&u.IsAdmin, &u.IsModerator, &u.Banned, &u.EmailVerified, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, apperrors.NotFoundError("User")
	}
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	return &u, nil
}

func (s *SQLiteStore) GetUserByEmail(email string) (*models.User, error) {
	return scanUser(s.db.QueryRow("SELECT "+userColumns+" FROM users WHERE LOWER(email) = LOWER(?)", email))
}

func (s *SQLiteStore) GetUserByID(id int) (*models.User, error) {
	return scanUser(s.db.QueryRow("SELECT "+userColumns+" FROM users WHERE id = ?", id))
}

func (s *SQLiteStore) GetUserByUsername(username string) (*models.User, error) {
	return scanUser(s.db.QueryRow("SELECT "+userColumns+" FROM users WHERE LOWER(username) = LOWER(?)", username))
}

func (s *SQLiteStore) UpdateUserAvatar(userID int, avatarURL string) error {
	_, err := s.db.Exec("UPDATE users SET avatar_url = ? WHERE id = ?", avatarURL, userID)
	return err
}

func (s *SQLiteStore) UpdateUserBio(userID int, bio string) error {
	_, err := s.db.Exec("UPDATE users SET bio = ? WHERE id = ?", bio, userID)
	return err
}

func (s *SQLiteStore) UpdateUserPassword(userID int, passwordHash string) error {
	_, err := s.db.Exec("UPDATE users SET password = ? WHERE id = ?", passwordHash, userID)
	return err
}

// ---- Account security operations ----

func (s *SQLiteStore) SetEmailVerified(userID int) error {
	_, err := s.db.Exec("UPDATE users SET email_verified = 1 WHERE id = ?", userID)
	return err
}

func (s *SQLiteStore) CreateEmailVerificationToken(userID int, token string, expiresAt time.Time) error {
	_, err := s.db.Exec("INSERT INTO email_verification_tokens (user_id, token, expires_at) VALUES (?, ?, ?)", userID, token, expiresAt)
	return err
}

// ConsumeEmailVerificationToken looks up and deletes the token in one call
// so it can't be replayed.
func (s *SQLiteStore) ConsumeEmailVerificationToken(token string) (int, error) {
	var userID int
	err := s.db.QueryRow(
		"SELECT user_id FROM email_verification_tokens WHERE token = ? AND expires_at > ?", token, time.Now(),
	).Scan(&userID)
	if err == sql.ErrNoRows {
		return 0, apperrors.NotFoundError("EmailVerificationToken")
	}
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	_, _ = s.db.Exec("DELETE FROM email_verification_tokens WHERE token = ?", token)
	return userID, nil
}

func (s *SQLiteStore) CreatePasswordResetToken(userID int, token string, expiresAt time.Time) error {
	_, err := s.db.Exec("INSERT INTO password_reset_tokens (user_id, token, expires_at) VALUES (?, ?, ?)", userID, token, expiresAt)
	return err
}

// ConsumePasswordResetToken validates a single-use token and marks it used
// so the same reset link can't be replayed.
func (s *SQLiteStore) ConsumePasswordResetToken(token string) (int, error) {
	var userID int
	err := s.db.QueryRow(
		"SELECT user_id FROM password_reset_tokens WHERE token = ? AND used = 0 AND expires_at > ?", token, time.Now(),
	).Scan(&userID)
	if err == sql.ErrNoRows {
		return 0, apperrors.NotFoundError("PasswordResetToken")
	}
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	if _, err := s.db.Exec("UPDATE password_reset_tokens SET used = 1 WHERE token = ?", token); err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	return userID, nil
}

// FindOrCreateOAuthUser links or creates a local account for an OAuth
// identity: an existing link wins, then an existing account matching the
// provider's email, then a brand new account with a de-duplicated username.
func (s *SQLiteStore) FindOrCreateOAuthUser(provider, providerUserID, email, preferredUsername, avatarURL string) (*models.User, error) {
	var userID int
	err := s.db.QueryRow(
		"SELECT user_id FROM oauth_identities WHERE provider = ? AND provider_user_id = ?", provider, providerUserID,
	).Scan(&userID)
	if err == nil {
		return s.GetUserByID(userID)
	}
	if err != sql.ErrNoRows {
		return nil, apperrors.DatabaseError(err)
	}

	if email == "" {
		email = fmt.Sprintf("%s-%s@users.noreply.%s", provider, providerUserID, provider)
	}

	if u, err := s.GetUserByEmail(email); err == nil {
		if _, err := s.db.Exec(
			"INSERT INTO oauth_identities (user_id, provider, provider_user_id) VALUES (?, ?, ?)", u.ID, provider, providerUserID,
		); err != nil {
			return nil, apperrors.DatabaseError(err)
		}
		return u, nil
	}

	username := preferredUsername
	for i := 0; ; i++ {
		if i > 0 {
			username = fmt.Sprintf("%s%d", preferredUsername, i)
		}
		if _, err := s.GetUserByUsername(username); err != nil {
			break
		}
	}

	// This can never match a real bcrypt hash, so password login is
	// impossible for OAuth-only accounts until they set one explicitly.
	placeholder := "oauth:" + provider + ":" + providerUserID
	newUserID, err := s.CreateUser(email, username, placeholder)
	if err != nil {
		return nil, err
	}
	if avatarURL != "" {
		_ = s.UpdateUserAvatar(newUserID, avatarURL)
	}
	_ = s.SetEmailVerified(newUserID) // the provider already verified it

	if _, err := s.db.Exec(
		"INSERT INTO oauth_identities (user_id, provider, provider_user_id) VALUES (?, ?, ?)", newUserID, provider, providerUserID,
	); err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	return s.GetUserByID(newUserID)
}

// ---- Admin operations ----

func (s *SQLiteStore) ListUsers(limit, offset int) ([]*models.User, error) {
	rows, err := s.db.Query("SELECT "+userColumns+" FROM users ORDER BY id ASC LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	defer rows.Close()
	var users []*models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.Password, &u.AvatarURL, &u.Bio,
			&u.IsAdmin, &u.IsModerator, &u.Banned, &u.EmailVerified, &u.CreatedAt); err != nil {
			return nil, apperrors.DatabaseError(err)
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}

func (s *SQLiteStore) CountUsers() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func (s *SQLiteStore) SetUserRole(userID int, isAdmin, isModerator bool) error {
	_, err := s.db.Exec("UPDATE users SET is_admin = ?, is_moderator = ? WHERE id = ?", isAdmin, isModerator, userID)
	return err
}

func (s *SQLiteStore) SetUserBanned(userID int, banned bool) error {
	_, err := s.db.Exec("UPDATE users SET banned = ? WHERE id = ?", banned, userID)
	return err
}

func (s *SQLiteStore) SetPostPinned(postID int, pinned bool) error {
	_, err := s.db.Exec("UPDATE posts SET pinned = ? WHERE id = ?", pinned, postID)
	return err
}

func (s *SQLiteStore) CreateReport(reporterID int, targetType string, targetID int, reason string) (int, error) {
	res, err := s.db.Exec(
		"INSERT INTO reports (reporter_id, target_type, target_id, reason) VALUES (?, ?, ?, ?)",
		reporterID, targetType, targetID, reason,
	)
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func (s *SQLiteStore) ListReports(status string, limit, offset int) ([]*models.Report, error) {
	rows, err := s.db.Query(`
		SELECT r.id, r.reporter_id, u.username, r.target_type, r.target_id, r.reason, r.status, r.created_at
		FROM reports r JOIN users u ON r.reporter_id = u.id
		WHERE r.status = ? ORDER BY r.created_at DESC LIMIT ? OFFSET ?`, status, limit, offset)
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	defer rows.Close()
	var reports []*models.Report
	for rows.Next() {
		var r models.Report
		if err := rows.Scan(&r.ID, &r.ReporterID, &r.ReporterUsername, &r.TargetType, &r.TargetID,
			&r.Reason, &r.Status, &r.CreatedAt); err != nil {
			return nil, apperrors.DatabaseError(err)
		}
		reports = append(reports, &r)
	}
	return reports, rows.Err()
}

func (s *SQLiteStore) CountOpenReports() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM reports WHERE status = 'open'").Scan(&count)
	return count, err
}

func (s *SQLiteStore) ResolveReport(id int) error {
	_, err := s.db.Exec("UPDATE reports SET status = 'resolved' WHERE id = ?", id)
	return err
}

// ---- Post operations ----

func (s *SQLiteStore) CreatePost(userID int, title, content string, categoryIDs []int, imageURL string) (int, error) {
	res, err := s.db.Exec("INSERT INTO posts (user_id, title, content, image_url, created_at) VALUES (?, ?, ?, ?, ?)", userID, title, content, imageURL, time.Now())
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
       COALESCE(p.image_url,''), p.pinned,
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
	s.loadTags(p)
	return p, nil
}

func (s *SQLiteStore) GetPosts(limit, offset int) ([]*models.Post, error) {
	return s.queryPosts(postBaseQuery+`
		GROUP BY p.id, u.username ORDER BY p.pinned DESC, p.created_at DESC LIMIT ? OFFSET ?`, limit, offset)
}

func (s *SQLiteStore) GetPostsByUser(userID, limit, offset int) ([]*models.Post, error) {
	return s.queryPosts(postBaseQuery+`
		WHERE p.user_id = ? GROUP BY p.id, u.username ORDER BY p.created_at DESC LIMIT ? OFFSET ?`, userID, limit, offset)
}

func (s *SQLiteStore) GetPostsByCategory(categoryID, limit, offset int) ([]*models.Post, error) {
	q := `
SELECT p.id, p.user_id, p.title, p.content, p.created_at, u.username,
       COALESCE(p.image_url,''), p.pinned,
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
       COALESCE(p.image_url,''), p.pinned,
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
		&p.ImageURL, &p.Pinned, &p.Likes, &p.Dislikes, &p.CommentCount)
	return &p, err
}

func (s *SQLiteStore) queryPosts(query string, args ...interface{}) ([]*models.Post, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}

	var posts []*models.Post
	for rows.Next() {
		var p models.Post
		if err := rows.Scan(&p.ID, &p.UserID, &p.Title, &p.Content, &p.CreatedAt, &p.Author,
			&p.ImageURL, &p.Pinned, &p.Likes, &p.Dislikes, &p.CommentCount); err != nil {
			rows.Close()
			return nil, apperrors.DatabaseError(err)
		}
		posts = append(posts, &p)
	}
	err = rows.Err()
	rows.Close() // release the connection before loadCategories opens another one

	// Loading categories per post after the fact (rather than inside the loop
	// above) avoids holding one connection open while requesting a second,
	// which would deadlock a pool capped at a single connection.
	for _, p := range posts {
		s.loadCategories(p)
		s.loadTags(p)
	}
	return posts, err
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

func (s *SQLiteStore) loadTags(p *models.Post) {
	rows, err := s.db.Query(
		"SELECT t.id, t.name FROM tags t JOIN post_tags pt ON t.id = pt.tag_id WHERE pt.post_id = ? ORDER BY t.name",
		p.ID,
	)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var t models.Tag
		_ = rows.Scan(&t.ID, &t.Name)
		p.Tags = append(p.Tags, t)
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
	return s.deletePost(id, "id = ? AND user_id = ?", id, userID)
}

// AdminDeletePost removes a post regardless of ownership - used by
// moderators/admins acting on a report.
func (s *SQLiteStore) AdminDeletePost(id int) error {
	return s.deletePost(id, "id = ?", id)
}

func (s *SQLiteStore) deletePost(id int, whereClause string, whereArgs ...interface{}) error {
	tx, err := s.db.Begin()
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	defer tx.Rollback()

	stmts := []struct {
		q    string
		args []interface{}
	}{
		{"DELETE FROM post_categories WHERE post_id = ?", []interface{}{id}},
		{"DELETE FROM post_tags WHERE post_id = ?", []interface{}{id}},
		{"DELETE FROM bookmarks WHERE post_id = ?", []interface{}{id}},
		{"DELETE FROM post_likes WHERE post_id = ?", []interface{}{id}},
		{"DELETE FROM comment_likes WHERE comment_id IN (SELECT id FROM comments WHERE post_id = ?)", []interface{}{id}},
		{"DELETE FROM comments WHERE post_id = ?", []interface{}{id}},
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s.q, s.args...); err != nil {
			return apperrors.DatabaseError(err)
		}
	}

	res, err := tx.Exec("DELETE FROM posts WHERE "+whereClause, whereArgs...)
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
	posts, err := s.queryPosts(postBaseQuery+`
		WHERE p.title LIKE ? OR p.content LIKE ?
		GROUP BY p.id, u.username ORDER BY p.created_at DESC LIMIT ? OFFSET ?`, like, like, limit, offset)
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

// ---- Tag operations ----

// SetPostTags replaces a post's tag set with tagNames, creating any tags
// that don't exist yet. Names are trimmed and de-duplicated case-insensitively.
func (s *SQLiteStore) SetPostTags(postID int, tagNames []string) error {
	seen := make(map[string]bool)
	var names []string
	for _, n := range tagNames {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		names = append(names, n)
	}

	if _, err := s.db.Exec("DELETE FROM post_tags WHERE post_id = ?", postID); err != nil {
		return apperrors.DatabaseError(err)
	}
	for _, name := range names {
		if _, err := s.db.Exec("INSERT OR IGNORE INTO tags (name) VALUES (?)", name); err != nil {
			return apperrors.DatabaseError(err)
		}
		var tagID int
		if err := s.db.QueryRow("SELECT id FROM tags WHERE name = ?", name).Scan(&tagID); err != nil {
			return apperrors.DatabaseError(err)
		}
		if _, err := s.db.Exec("INSERT INTO post_tags (post_id, tag_id) VALUES (?, ?)", postID, tagID); err != nil {
			return apperrors.DatabaseError(err)
		}
	}
	return nil
}

func (s *SQLiteStore) GetPostsByTag(tagName string, limit, offset int) ([]*models.Post, error) {
	q := `
SELECT p.id, p.user_id, p.title, p.content, p.created_at, u.username,
       COALESCE(p.image_url,''), p.pinned,
       COALESCE(SUM(CASE WHEN pl.is_like = 1 THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN pl.is_like = 0 THEN 1 ELSE 0 END), 0),
       COUNT(DISTINCT c.id)
FROM posts p
JOIN users u ON p.user_id = u.id
JOIN post_tags pt ON p.id = pt.post_id
JOIN tags t ON pt.tag_id = t.id
LEFT JOIN post_likes pl ON p.id = pl.post_id
LEFT JOIN comments c ON p.id = c.post_id
WHERE LOWER(t.name) = LOWER(?)
GROUP BY p.id, u.username ORDER BY p.created_at DESC LIMIT ? OFFSET ?`
	return s.queryPosts(q, tagName, limit, offset)
}

func (s *SQLiteStore) CountPostsByTag(tagName string) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(DISTINCT p.id) FROM posts p
		JOIN post_tags pt ON p.id = pt.post_id
		JOIN tags t ON pt.tag_id = t.id
		WHERE LOWER(t.name) = LOWER(?)`, tagName).Scan(&count)
	return count, err
}

func (s *SQLiteStore) SearchTags(prefix string, limit int) ([]*models.Tag, error) {
	rows, err := s.db.Query("SELECT id, name FROM tags WHERE name LIKE ? ORDER BY name LIMIT ?", prefix+"%", limit)
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	defer rows.Close()
	var tags []*models.Tag
	for rows.Next() {
		var t models.Tag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, apperrors.DatabaseError(err)
		}
		tags = append(tags, &t)
	}
	return tags, rows.Err()
}

// ---- Bookmark operations ----

func (s *SQLiteStore) ToggleBookmark(postID, userID int, bookmarked bool) error {
	if bookmarked {
		_, err := s.db.Exec("INSERT OR IGNORE INTO bookmarks (user_id, post_id) VALUES (?, ?)", userID, postID)
		return err
	}
	_, err := s.db.Exec("DELETE FROM bookmarks WHERE user_id = ? AND post_id = ?", userID, postID)
	return err
}

func (s *SQLiteStore) IsBookmarked(postID, userID int) (bool, error) {
	var exists int
	err := s.db.QueryRow("SELECT 1 FROM bookmarks WHERE user_id = ? AND post_id = ?", userID, postID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *SQLiteStore) GetBookmarkedPostsByUser(userID, limit, offset int) ([]*models.Post, error) {
	q := `
SELECT p.id, p.user_id, p.title, p.content, p.created_at, u.username,
       COALESCE(p.image_url,''), p.pinned,
       COALESCE(SUM(CASE WHEN pl.is_like = 1 THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN pl.is_like = 0 THEN 1 ELSE 0 END), 0),
       COUNT(DISTINCT c.id)
FROM posts p
JOIN users u ON p.user_id = u.id
JOIN bookmarks b ON p.id = b.post_id AND b.user_id = ?
LEFT JOIN post_likes pl ON p.id = pl.post_id
LEFT JOIN comments c ON p.id = c.post_id
GROUP BY p.id, u.username ORDER BY b.created_at DESC LIMIT ? OFFSET ?`
	return s.queryPosts(q, userID, limit, offset)
}

func (s *SQLiteStore) CountBookmarkedPostsByUser(userID int) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM bookmarks WHERE user_id = ?", userID).Scan(&count)
	return count, err
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
		SELECT c.id, c.post_id, c.user_id, c.content, c.created_at, u.username,
			(SELECT COUNT(*) FROM comment_likes WHERE comment_id = c.id AND is_like = 1),
			(SELECT COUNT(*) FROM comment_likes WHERE comment_id = c.id AND is_like = 0)
		FROM comments c JOIN users u ON c.user_id = u.id
		WHERE c.post_id = ? ORDER BY c.created_at ASC`, postID)
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	defer rows.Close()
	var comments []*models.Comment
	for rows.Next() {
		var c models.Comment
		if err := rows.Scan(&c.ID, &c.PostID, &c.UserID, &c.Content, &c.CreatedAt, &c.Author, &c.Likes, &c.Dislikes); err != nil {
			return nil, apperrors.DatabaseError(err)
		}
		comments = append(comments, &c)
	}
	return comments, rows.Err()
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
	return s.deleteComment(id, "id = ? AND user_id = ?", id, userID)
}

// AdminDeleteComment removes a comment regardless of ownership.
func (s *SQLiteStore) AdminDeleteComment(id int) error {
	return s.deleteComment(id, "id = ?", id)
}

func (s *SQLiteStore) deleteComment(id int, whereClause string, whereArgs ...interface{}) error {
	_, _ = s.db.Exec("DELETE FROM comment_likes WHERE comment_id = ?", id)
	res, err := s.db.Exec("DELETE FROM comments WHERE "+whereClause, whereArgs...)
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
	err := s.db.QueryRow("SELECT id, user_id, token, expires_at FROM refresh_tokens WHERE token = ? AND expires_at > ?", token, time.Now()).
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

// ---- Notification operations ----

func (s *SQLiteStore) CreateNotification(userID int, notifType, payload string) (int, error) {
	res, err := s.db.Exec("INSERT INTO notifications (user_id, type, payload) VALUES (?, ?, ?)", userID, notifType, payload)
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func (s *SQLiteStore) GetNotificationsByUser(userID, limit, offset int) ([]*models.NotificationRecord, error) {
	rows, err := s.db.Query(
		"SELECT id, user_id, type, payload, read, created_at FROM notifications WHERE user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?",
		userID, limit, offset,
	)
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	defer rows.Close()
	var notifs []*models.NotificationRecord
	for rows.Next() {
		var n models.NotificationRecord
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Payload, &n.Read, &n.CreatedAt); err != nil {
			return nil, apperrors.DatabaseError(err)
		}
		notifs = append(notifs, &n)
	}
	return notifs, rows.Err()
}

func (s *SQLiteStore) CountUnreadNotifications(userID int) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read = 0", userID).Scan(&count)
	return count, err
}

func (s *SQLiteStore) MarkNotificationRead(id, userID int) error {
	_, err := s.db.Exec("UPDATE notifications SET read = 1 WHERE id = ? AND user_id = ?", id, userID)
	return err
}

func (s *SQLiteStore) MarkAllNotificationsRead(userID int) error {
	_, err := s.db.Exec("UPDATE notifications SET read = 1 WHERE user_id = ? AND read = 0", userID)
	return err
}

// ---- Utility ----

func (s *SQLiteStore) Close() error  { return s.db.Close() }
func (s *SQLiteStore) Health() error { return s.db.Ping() }
