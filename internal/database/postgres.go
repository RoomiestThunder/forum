package database

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"
	"strings"
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
	// The very first account on a fresh install becomes an admin - there's
	// no other bootstrap mechanism (no seed script, no CLI), so without this
	// nobody could ever reach the admin panel.
	var userCount int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount); err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	var id int
	err := s.db.QueryRow(
		"INSERT INTO users (email, username, password, is_admin) VALUES ($1, $2, $3, $4) RETURNING id",
		email, username, passwordHash, userCount == 0,
	).Scan(&id)
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	return id, nil
}

const pgUserColumns = "id, email, username, password, COALESCE(avatar_url,''), COALESCE(bio,''), is_admin, is_moderator, banned, email_verified, created_at"

func scanPgUser(row *sql.Row) (*models.User, error) {
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

func (s *PostgresStore) GetUserByEmail(email string) (*models.User, error) {
	return scanPgUser(s.db.QueryRow("SELECT "+pgUserColumns+" FROM users WHERE LOWER(email) = LOWER($1)", email))
}

func (s *PostgresStore) GetUserByID(id int) (*models.User, error) {
	return scanPgUser(s.db.QueryRow("SELECT "+pgUserColumns+" FROM users WHERE id = $1", id))
}

func (s *PostgresStore) GetUserByUsername(username string) (*models.User, error) {
	return scanPgUser(s.db.QueryRow("SELECT "+pgUserColumns+" FROM users WHERE LOWER(username) = LOWER($1)", username))
}

func (s *PostgresStore) UpdateUserAvatar(userID int, avatarURL string) error {
	_, err := s.db.Exec("UPDATE users SET avatar_url = $1 WHERE id = $2", avatarURL, userID)
	return err
}

func (s *PostgresStore) UpdateUserBio(userID int, bio string) error {
	_, err := s.db.Exec("UPDATE users SET bio = $1 WHERE id = $2", bio, userID)
	return err
}

func (s *PostgresStore) UpdateUserPassword(userID int, passwordHash string) error {
	_, err := s.db.Exec("UPDATE users SET password = $1 WHERE id = $2", passwordHash, userID)
	return err
}

// ---- Account security operations ----

func (s *PostgresStore) SetEmailVerified(userID int) error {
	_, err := s.db.Exec("UPDATE users SET email_verified = TRUE WHERE id = $1", userID)
	return err
}

func (s *PostgresStore) CreateEmailVerificationToken(userID int, token string, expiresAt time.Time) error {
	_, err := s.db.Exec("INSERT INTO email_verification_tokens (user_id, token, expires_at) VALUES ($1, $2, $3)", userID, token, expiresAt)
	return err
}

func (s *PostgresStore) ConsumeEmailVerificationToken(token string) (int, error) {
	var userID int
	err := s.db.QueryRow(
		"SELECT user_id FROM email_verification_tokens WHERE token = $1 AND expires_at > NOW()", token,
	).Scan(&userID)
	if err == sql.ErrNoRows {
		return 0, apperrors.NotFoundError("EmailVerificationToken")
	}
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	_, _ = s.db.Exec("DELETE FROM email_verification_tokens WHERE token = $1", token)
	return userID, nil
}

func (s *PostgresStore) CreatePasswordResetToken(userID int, token string, expiresAt time.Time) error {
	_, err := s.db.Exec("INSERT INTO password_reset_tokens (user_id, token, expires_at) VALUES ($1, $2, $3)", userID, token, expiresAt)
	return err
}

func (s *PostgresStore) ConsumePasswordResetToken(token string) (int, error) {
	var userID int
	err := s.db.QueryRow(
		"SELECT user_id FROM password_reset_tokens WHERE token = $1 AND used = FALSE AND expires_at > NOW()", token,
	).Scan(&userID)
	if err == sql.ErrNoRows {
		return 0, apperrors.NotFoundError("PasswordResetToken")
	}
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	if _, err := s.db.Exec("UPDATE password_reset_tokens SET used = TRUE WHERE token = $1", token); err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	return userID, nil
}

func (s *PostgresStore) FindOrCreateOAuthUser(provider, providerUserID, email, preferredUsername, avatarURL string) (*models.User, error) {
	var userID int
	err := s.db.QueryRow(
		"SELECT user_id FROM oauth_identities WHERE provider = $1 AND provider_user_id = $2", provider, providerUserID,
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
			"INSERT INTO oauth_identities (user_id, provider, provider_user_id) VALUES ($1, $2, $3)", u.ID, provider, providerUserID,
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

	placeholder := "oauth:" + provider + ":" + providerUserID
	newUserID, err := s.CreateUser(email, username, placeholder)
	if err != nil {
		return nil, err
	}
	if avatarURL != "" {
		_ = s.UpdateUserAvatar(newUserID, avatarURL)
	}
	_ = s.SetEmailVerified(newUserID)

	if _, err := s.db.Exec(
		"INSERT INTO oauth_identities (user_id, provider, provider_user_id) VALUES ($1, $2, $3)", newUserID, provider, providerUserID,
	); err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	return s.GetUserByID(newUserID)
}

// ---- Admin operations ----

func (s *PostgresStore) ListUsers(limit, offset int) ([]*models.User, error) {
	rows, err := s.db.Query("SELECT "+pgUserColumns+" FROM users ORDER BY id ASC LIMIT $1 OFFSET $2", limit, offset)
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

func (s *PostgresStore) CountUsers() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func (s *PostgresStore) SetUserRole(userID int, isAdmin, isModerator bool) error {
	_, err := s.db.Exec("UPDATE users SET is_admin = $1, is_moderator = $2 WHERE id = $3", isAdmin, isModerator, userID)
	return err
}

func (s *PostgresStore) SetUserBanned(userID int, banned bool) error {
	_, err := s.db.Exec("UPDATE users SET banned = $1 WHERE id = $2", banned, userID)
	return err
}

func (s *PostgresStore) SetPostPinned(postID int, pinned bool) error {
	_, err := s.db.Exec("UPDATE posts SET pinned = $1 WHERE id = $2", pinned, postID)
	return err
}

func (s *PostgresStore) CreateReport(reporterID int, targetType string, targetID int, reason string) (int, error) {
	var id int
	err := s.db.QueryRow(
		"INSERT INTO reports (reporter_id, target_type, target_id, reason) VALUES ($1, $2, $3, $4) RETURNING id",
		reporterID, targetType, targetID, reason,
	).Scan(&id)
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	return id, nil
}

func (s *PostgresStore) ListReports(status string, limit, offset int) ([]*models.Report, error) {
	rows, err := s.db.Query(`
		SELECT r.id, r.reporter_id, u.username, r.target_type, r.target_id, r.reason, r.status, r.created_at
		FROM reports r JOIN users u ON r.reporter_id = u.id
		WHERE r.status = $1 ORDER BY r.created_at DESC LIMIT $2 OFFSET $3`, status, limit, offset)
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

func (s *PostgresStore) CountOpenReports() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM reports WHERE status = 'open'").Scan(&count)
	return count, err
}

func (s *PostgresStore) ResolveReport(id int) error {
	_, err := s.db.Exec("UPDATE reports SET status = 'resolved' WHERE id = $1", id)
	return err
}

// ---- Post operations ----

func (s *PostgresStore) CreatePost(userID int, title, content string, categoryIDs []int, imageURL string) (int, error) {
	var postID int
	err := s.db.QueryRow(
		"INSERT INTO posts (user_id, title, content, image_url, created_at) VALUES ($1, $2, $3, $4, $5) RETURNING id",
		userID, title, content, imageURL, time.Now(),
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
       COALESCE(p.image_url,''), p.pinned,
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
			&p.ImageURL, &p.Pinned, &p.Likes, &p.Dislikes, &p.CommentCount)
	if err == sql.ErrNoRows {
		return nil, apperrors.NotFoundError("Post")
	}
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	s.loadCategories(&p)
	s.loadTags(&p)
	return &p, nil
}

func (s *PostgresStore) GetPosts(limit, offset int) ([]*models.Post, error) {
	return s.queryPosts(pgPostBase+`
		GROUP BY p.id, u.username ORDER BY p.pinned DESC, p.created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
}

func (s *PostgresStore) GetPostsByUser(userID, limit, offset int) ([]*models.Post, error) {
	return s.queryPosts(pgPostBase+`
		WHERE p.user_id = $1 GROUP BY p.id, u.username ORDER BY p.created_at DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset)
}

func (s *PostgresStore) GetPostsByCategory(categoryID, limit, offset int) ([]*models.Post, error) {
	q := `
SELECT p.id, p.user_id, p.title, p.content, p.created_at, u.username,
       COALESCE(p.image_url,''), p.pinned,
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
       COALESCE(p.image_url,''), p.pinned,
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
	rows.Close() // release the connection before loadCategories requests another one

	for _, p := range posts {
		s.loadCategories(p)
		s.loadTags(p)
	}
	return posts, err
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

func (s *PostgresStore) loadTags(p *models.Post) {
	rows, err := s.db.Query(
		"SELECT t.id, t.name FROM tags t JOIN post_tags pt ON t.id = pt.tag_id WHERE pt.post_id = $1 ORDER BY t.name",
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
	return s.deletePost(id, "id = $1 AND user_id = $2", id, userID)
}

// AdminDeletePost removes a post regardless of ownership - used by
// moderators/admins acting on a report.
func (s *PostgresStore) AdminDeletePost(id int) error {
	return s.deletePost(id, "id = $1", id)
}

func (s *PostgresStore) deletePost(id int, whereClause string, whereArgs ...interface{}) error {
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
		{"DELETE FROM post_tags WHERE post_id = $1", []interface{}{id}},
		{"DELETE FROM bookmarks WHERE post_id = $1", []interface{}{id}},
		{"DELETE FROM post_likes WHERE post_id = $1", []interface{}{id}},
		{"DELETE FROM comment_likes WHERE comment_id IN (SELECT id FROM comments WHERE post_id = $1)", []interface{}{id}},
		{"DELETE FROM comments WHERE post_id = $1", []interface{}{id}},
	}
	for _, st := range stmts {
		if _, err := tx.Exec(st.q, st.args...); err != nil {
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

// ---- Tag operations ----

func (s *PostgresStore) SetPostTags(postID int, tagNames []string) error {
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

	if _, err := s.db.Exec("DELETE FROM post_tags WHERE post_id = $1", postID); err != nil {
		return apperrors.DatabaseError(err)
	}
	for _, name := range names {
		var tagID int
		err := s.db.QueryRow(
			"INSERT INTO tags (name) VALUES ($1) ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id",
			name,
		).Scan(&tagID)
		if err != nil {
			return apperrors.DatabaseError(err)
		}
		if _, err := s.db.Exec("INSERT INTO post_tags (post_id, tag_id) VALUES ($1, $2)", postID, tagID); err != nil {
			return apperrors.DatabaseError(err)
		}
	}
	return nil
}

func (s *PostgresStore) GetPostsByTag(tagName string, limit, offset int) ([]*models.Post, error) {
	q := `
SELECT p.id, p.user_id, p.title, p.content, p.created_at, u.username,
       COALESCE(p.image_url,''), p.pinned,
       COALESCE(SUM(CASE WHEN pl.is_like = TRUE  THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN pl.is_like = FALSE THEN 1 ELSE 0 END), 0),
       COUNT(DISTINCT c.id)
FROM posts p
JOIN users u ON p.user_id = u.id
JOIN post_tags pt ON p.id = pt.post_id
JOIN tags t ON pt.tag_id = t.id
LEFT JOIN post_likes pl ON p.id = pl.post_id
LEFT JOIN comments c ON p.id = c.post_id
WHERE LOWER(t.name) = LOWER($1)
GROUP BY p.id, u.username ORDER BY p.created_at DESC LIMIT $2 OFFSET $3`
	return s.queryPosts(q, tagName, limit, offset)
}

func (s *PostgresStore) CountPostsByTag(tagName string) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(DISTINCT p.id) FROM posts p
		JOIN post_tags pt ON p.id = pt.post_id
		JOIN tags t ON pt.tag_id = t.id
		WHERE LOWER(t.name) = LOWER($1)`, tagName).Scan(&count)
	return count, err
}

func (s *PostgresStore) SearchTags(prefix string, limit int) ([]*models.Tag, error) {
	rows, err := s.db.Query("SELECT id, name FROM tags WHERE name ILIKE $1 ORDER BY name LIMIT $2", prefix+"%", limit)
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

func (s *PostgresStore) ToggleBookmark(postID, userID int, bookmarked bool) error {
	if bookmarked {
		_, err := s.db.Exec("INSERT INTO bookmarks (user_id, post_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", userID, postID)
		return err
	}
	_, err := s.db.Exec("DELETE FROM bookmarks WHERE user_id = $1 AND post_id = $2", userID, postID)
	return err
}

func (s *PostgresStore) IsBookmarked(postID, userID int) (bool, error) {
	var exists int
	err := s.db.QueryRow("SELECT 1 FROM bookmarks WHERE user_id = $1 AND post_id = $2", userID, postID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *PostgresStore) GetBookmarkedPostsByUser(userID, limit, offset int) ([]*models.Post, error) {
	q := `
SELECT p.id, p.user_id, p.title, p.content, p.created_at, u.username,
       COALESCE(p.image_url,''), p.pinned,
       COALESCE(SUM(CASE WHEN pl.is_like = TRUE  THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN pl.is_like = FALSE THEN 1 ELSE 0 END), 0),
       COUNT(DISTINCT c.id)
FROM posts p
JOIN users u ON p.user_id = u.id
JOIN bookmarks b ON p.id = b.post_id AND b.user_id = $1
LEFT JOIN post_likes pl ON p.id = pl.post_id
LEFT JOIN comments c ON p.id = c.post_id
GROUP BY p.id, u.username, b.created_at ORDER BY b.created_at DESC LIMIT $2 OFFSET $3`
	return s.queryPosts(q, userID, limit, offset)
}

func (s *PostgresStore) CountBookmarkedPostsByUser(userID int) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM bookmarks WHERE user_id = $1", userID).Scan(&count)
	return count, err
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
		SELECT c.id, c.post_id, c.user_id, c.content, c.created_at, u.username,
			(SELECT COUNT(*) FROM comment_likes WHERE comment_id = c.id AND is_like = TRUE),
			(SELECT COUNT(*) FROM comment_likes WHERE comment_id = c.id AND is_like = FALSE)
		FROM comments c JOIN users u ON c.user_id = u.id
		WHERE c.post_id = $1 ORDER BY c.created_at ASC`, postID)
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
	return s.deleteComment(id, "id = $1 AND user_id = $2", id, userID)
}

// AdminDeleteComment removes a comment regardless of ownership.
func (s *PostgresStore) AdminDeleteComment(id int) error {
	return s.deleteComment(id, "id = $1", id)
}

func (s *PostgresStore) deleteComment(id int, whereClause string, whereArgs ...interface{}) error {
	_, _ = s.db.Exec("DELETE FROM comment_likes WHERE comment_id = $1", id)
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
	err := s.db.QueryRow("SELECT id, user_id, token, expires_at FROM refresh_tokens WHERE token = $1 AND expires_at > NOW()", token).
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

// ---- Notification operations ----

func (s *PostgresStore) CreateNotification(userID int, notifType, payload string) (int, error) {
	var id int
	err := s.db.QueryRow(
		"INSERT INTO notifications (user_id, type, payload) VALUES ($1, $2, $3) RETURNING id",
		userID, notifType, payload,
	).Scan(&id)
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	return id, nil
}

func (s *PostgresStore) GetNotificationsByUser(userID, limit, offset int) ([]*models.NotificationRecord, error) {
	rows, err := s.db.Query(
		"SELECT id, user_id, type, payload, read, created_at FROM notifications WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3",
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

func (s *PostgresStore) CountUnreadNotifications(userID int) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read = FALSE", userID).Scan(&count)
	return count, err
}

func (s *PostgresStore) MarkNotificationRead(id, userID int) error {
	_, err := s.db.Exec("UPDATE notifications SET read = TRUE WHERE id = $1 AND user_id = $2", id, userID)
	return err
}

func (s *PostgresStore) MarkAllNotificationsRead(userID int) error {
	_, err := s.db.Exec("UPDATE notifications SET read = TRUE WHERE user_id = $1 AND read = FALSE", userID)
	return err
}

// ---- Utility ----

func (s *PostgresStore) Close() error  { return s.db.Close() }
func (s *PostgresStore) Health() error { return s.db.Ping() }
