package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	apperrors "forum/internal/errors"
	"forum/internal/models"

	"github.com/mattn/go-sqlite3"
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
		draft BOOLEAN NOT NULL DEFAULT 0,
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
		parent_id INTEGER,
		depth INTEGER NOT NULL DEFAULT 0,
		deleted BOOLEAN NOT NULL DEFAULT 0,
		FOREIGN KEY(post_id) REFERENCES posts(id),
		FOREIGN KEY(user_id) REFERENCES users(id),
		FOREIGN KEY(parent_id) REFERENCES comments(id)
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
	CREATE TABLE IF NOT EXISTS conversations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_a_id INTEGER NOT NULL,
		user_b_id INTEGER NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_message_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_a_id) REFERENCES users(id),
		FOREIGN KEY(user_b_id) REFERENCES users(id),
		UNIQUE(user_a_id, user_b_id)
	);
	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		conversation_id INTEGER NOT NULL,
		sender_id INTEGER NOT NULL,
		content TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		read_at DATETIME,
		FOREIGN KEY(conversation_id) REFERENCES conversations(id),
		FOREIGN KEY(sender_id) REFERENCES users(id)
	);
	CREATE INDEX IF NOT EXISTS idx_messages_conversation ON messages(conversation_id);
	CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		actor_id INTEGER NOT NULL,
		action TEXT NOT NULL,
		target_type TEXT NOT NULL,
		target_id INTEGER NOT NULL,
		details TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(actor_id) REFERENCES users(id)
	);
	CREATE INDEX IF NOT EXISTS audit_log_created_idx ON audit_log(created_at DESC, id DESC);
	CREATE TABLE IF NOT EXISTS user_blocks (
		blocker_id INTEGER NOT NULL,
		blocked_id INTEGER NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (blocker_id, blocked_id),
		FOREIGN KEY(blocker_id) REFERENCES users(id),
		FOREIGN KEY(blocked_id) REFERENCES users(id),
		CHECK (blocker_id <> blocked_id)
	);
	`)
	if err != nil {
		return err
	}
	if err := sqliteAddMissingColumns(db); err != nil {
		return err
	}
	// Created after the columns exist, so they also work on a database that
	// predates them.
	if _, err := db.Exec(`
	CREATE INDEX IF NOT EXISTS comments_post_idx ON comments(post_id, created_at, id);
	CREATE INDEX IF NOT EXISTS comments_parent_idx ON comments(parent_id);
	`); err != nil {
		return err
	}
	// username was never unique. Rename any existing duplicates (the oldest
	// account keeps the name; later ones get their id appended) so the index
	// can be built; see migration 013.
	if _, err := db.Exec(`
	UPDATE users SET username = username || '_' || id
	WHERE EXISTS (
		SELECT 1 FROM users o
		WHERE LOWER(o.username) = LOWER(users.username) AND o.id < users.id
	);
	CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower_idx ON users (LOWER(username));
	`); err != nil {
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

// isSQLiteUniqueViolation reports whether err is a UNIQUE constraint failure.
func isSQLiteUniqueViolation(err error) bool {
	var sqliteErr sqlite3.Error
	return errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique
}

// sqliteLateColumns are columns that were added to a table after its first
// version (the PostgreSQL counterparts are migrations 002-011). CREATE TABLE IF
// NOT EXISTS leaves an existing table as it is, so a database file created by an
// older build would otherwise fail with "no such column" after an upgrade.
//
// SQLite only allows ADD COLUMN with a constant default, which is why
// users.created_at gets a fixed epoch rather than CURRENT_TIMESTAMP.
var sqliteLateColumns = []struct{ table, column, definition string }{
	{"users", "bio", "TEXT NOT NULL DEFAULT ''"},
	{"users", "created_at", "DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00'"},
	{"users", "is_admin", "BOOLEAN NOT NULL DEFAULT 0"},
	{"users", "is_moderator", "BOOLEAN NOT NULL DEFAULT 0"},
	{"users", "banned", "BOOLEAN NOT NULL DEFAULT 0"},
	{"users", "email_verified", "BOOLEAN NOT NULL DEFAULT 0"},
	{"posts", "pinned", "BOOLEAN NOT NULL DEFAULT 0"},
	{"posts", "draft", "BOOLEAN NOT NULL DEFAULT 0"},
	{"comments", "parent_id", "INTEGER REFERENCES comments(id)"},
	{"comments", "depth", "INTEGER NOT NULL DEFAULT 0"},
	{"comments", "deleted", "BOOLEAN NOT NULL DEFAULT 0"},
}

// sqliteAddMissingColumns brings a database created by an older build up to
// the current column set. It is idempotent: on a fresh database, where CREATE
// TABLE already defined every column, it does nothing.
func sqliteAddMissingColumns(db *sql.DB) error {
	for _, c := range sqliteLateColumns {
		var present int
		if err := db.QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", c.table, c.column,
		).Scan(&present); err != nil {
			return fmt.Errorf("inspect %s.%s: %w", c.table, c.column, err)
		}
		if present > 0 {
			continue
		}
		stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", c.table, c.column, c.definition)
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("add %s.%s: %w", c.table, c.column, err)
		}
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
		if isSQLiteUniqueViolation(err) {
			return 0, errAccountTaken()
		}
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

// SearchUsers powers @mention autocomplete - a case-insensitive username
// prefix match. LIKE is case-insensitive for ASCII in SQLite by default.
func (s *SQLiteStore) SearchUsers(prefix string, limit int) ([]*models.UserSummary, error) {
	rows, err := s.db.Query(
		"SELECT id, username, avatar_url FROM users WHERE username LIKE ? ORDER BY username LIMIT ?",
		prefix+"%", limit,
	)
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	defer rows.Close()
	var users []*models.UserSummary
	for rows.Next() {
		var u models.UserSummary
		if err := rows.Scan(&u.ID, &u.Username, &u.AvatarURL); err != nil {
			return nil, apperrors.DatabaseError(err)
		}
		users = append(users, &u)
	}
	return users, rows.Err()
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

func (s *SQLiteStore) CreatePost(userID int, title, content string, categoryIDs []int, imageURL string, draft bool) (int, error) {
	res, err := s.db.Exec("INSERT INTO posts (user_id, title, content, image_url, draft, created_at) VALUES (?, ?, ?, ?, ?, ?)", userID, title, content, imageURL, draft, time.Now())
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

// SetPostDraft flips a post's draft flag - used to publish a draft (draft
// -> false). Only the owner may do this.
func (s *SQLiteStore) SetPostDraft(id, userID int, draft bool) error {
	res, err := s.db.Exec("UPDATE posts SET draft = ? WHERE id = ? AND user_id = ?", draft, id, userID)
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return apperrors.PermissionError("update post")
	}
	return nil
}

// postBaseQuery is the common SELECT for posts with inline aggregated counts.
// This eliminates the N+1 fillPostMeta pattern.
const postBaseQuery = `
SELECT p.id, p.user_id, p.title, p.content, p.created_at, u.username,
       COALESCE(p.image_url,''), p.pinned, p.draft,
       COALESCE(COUNT(DISTINCT CASE WHEN pl.is_like = 1 THEN pl.id END), 0) AS likes,
       COALESCE(COUNT(DISTINCT CASE WHEN pl.is_like = 0 THEN pl.id END), 0) AS dislikes,
       COUNT(DISTINCT c.id) AS comment_count
FROM posts p
JOIN users u ON p.user_id = u.id
LEFT JOIN post_likes pl ON p.id = pl.post_id
LEFT JOIN comments c ON p.id = c.post_id`

// GetPost fetches a post by id regardless of draft status - callers are
// responsible for checking Draft/UserID before showing it to anyone but the
// owner (see getPost in the API layer).
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
		WHERE p.draft = 0
		GROUP BY p.id, u.username ORDER BY p.pinned DESC, p.created_at DESC LIMIT ? OFFSET ?`, limit, offset)
}

func (s *SQLiteStore) GetPostsByUser(userID, limit, offset int) ([]*models.Post, error) {
	return s.queryPosts(postBaseQuery+`
		WHERE p.user_id = ? AND p.draft = 0 GROUP BY p.id, u.username ORDER BY p.created_at DESC LIMIT ? OFFSET ?`, userID, limit, offset)
}

func (s *SQLiteStore) GetDraftPostsByUser(userID, limit, offset int) ([]*models.Post, error) {
	return s.queryPosts(postBaseQuery+`
		WHERE p.user_id = ? AND p.draft = 1 GROUP BY p.id, u.username ORDER BY p.created_at DESC LIMIT ? OFFSET ?`, userID, limit, offset)
}

func (s *SQLiteStore) GetPostsByCategory(categoryID, limit, offset int) ([]*models.Post, error) {
	q := `
SELECT p.id, p.user_id, p.title, p.content, p.created_at, u.username,
       COALESCE(p.image_url,''), p.pinned, p.draft,
       COALESCE(COUNT(DISTINCT CASE WHEN pl.is_like = 1 THEN pl.id END), 0),
       COALESCE(COUNT(DISTINCT CASE WHEN pl.is_like = 0 THEN pl.id END), 0),
       COUNT(DISTINCT c.id)
FROM posts p
JOIN users u ON p.user_id = u.id
JOIN post_categories pc ON p.id = pc.post_id
LEFT JOIN post_likes pl ON p.id = pl.post_id
LEFT JOIN comments c ON p.id = c.post_id
WHERE pc.category_id = ? AND p.draft = 0
GROUP BY p.id, u.username ORDER BY p.created_at DESC LIMIT ? OFFSET ?`
	return s.queryPosts(q, categoryID, limit, offset)
}

func (s *SQLiteStore) GetLikedPostsByUser(userID, limit, offset int) ([]*models.Post, error) {
	q := `
SELECT p.id, p.user_id, p.title, p.content, p.created_at, u.username,
       COALESCE(p.image_url,''), p.pinned, p.draft,
       COALESCE(COUNT(DISTINCT CASE WHEN pl2.is_like = 1 THEN pl2.id END), 0),
       COALESCE(COUNT(DISTINCT CASE WHEN pl2.is_like = 0 THEN pl2.id END), 0),
       COUNT(DISTINCT c.id)
FROM posts p
JOIN users u ON p.user_id = u.id
JOIN post_likes liked ON p.id = liked.post_id AND liked.user_id = ? AND liked.is_like = 1
LEFT JOIN post_likes pl2 ON p.id = pl2.post_id
LEFT JOIN comments c ON p.id = c.post_id
WHERE p.draft = 0
GROUP BY p.id, u.username ORDER BY p.created_at DESC LIMIT ? OFFSET ?`
	return s.queryPosts(q, userID, limit, offset)
}

func (s *SQLiteStore) scanPost(row *sql.Row) (*models.Post, error) {
	var p models.Post
	err := row.Scan(&p.ID, &p.UserID, &p.Title, &p.Content, &p.CreatedAt, &p.Author,
		&p.ImageURL, &p.Pinned, &p.Draft, &p.Likes, &p.Dislikes, &p.CommentCount)
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
			&p.ImageURL, &p.Pinned, &p.Draft, &p.Likes, &p.Dislikes, &p.CommentCount); err != nil {
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
	return n, s.db.QueryRow("SELECT COUNT(*) FROM posts WHERE user_id = ? AND draft = 0", userID).Scan(&n)
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
	if err := s.db.QueryRow("SELECT COUNT(*) FROM posts WHERE (title LIKE ? OR content LIKE ?) AND draft = 0", like, like).Scan(&total); err != nil {
		return nil, 0, apperrors.DatabaseError(err)
	}
	posts, err := s.queryPosts(postBaseQuery+`
		WHERE (p.title LIKE ? OR p.content LIKE ?) AND p.draft = 0
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
       COALESCE(p.image_url,''), p.pinned, p.draft,
       COALESCE(COUNT(DISTINCT CASE WHEN pl.is_like = 1 THEN pl.id END), 0),
       COALESCE(COUNT(DISTINCT CASE WHEN pl.is_like = 0 THEN pl.id END), 0),
       COUNT(DISTINCT c.id)
FROM posts p
JOIN users u ON p.user_id = u.id
JOIN post_tags pt ON p.id = pt.post_id
JOIN tags t ON pt.tag_id = t.id
LEFT JOIN post_likes pl ON p.id = pl.post_id
LEFT JOIN comments c ON p.id = c.post_id
WHERE LOWER(t.name) = LOWER(?) AND p.draft = 0
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
       COALESCE(p.image_url,''), p.pinned, p.draft,
       COALESCE(COUNT(DISTINCT CASE WHEN pl.is_like = 1 THEN pl.id END), 0),
       COALESCE(COUNT(DISTINCT CASE WHEN pl.is_like = 0 THEN pl.id END), 0),
       COUNT(DISTINCT c.id)
FROM posts p
JOIN users u ON p.user_id = u.id
JOIN bookmarks b ON p.id = b.post_id AND b.user_id = ?
LEFT JOIN post_likes pl ON p.id = pl.post_id
LEFT JOIN comments c ON p.id = c.post_id
WHERE p.draft = 0
GROUP BY p.id, u.username ORDER BY b.created_at DESC LIMIT ? OFFSET ?`
	return s.queryPosts(q, userID, limit, offset)
}

func (s *SQLiteStore) CountBookmarkedPostsByUser(userID int) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM bookmarks WHERE user_id = ?", userID).Scan(&count)
	return count, err
}

// ---- Comment operations ----

func (s *SQLiteStore) CreateComment(postID, userID int, parentID *int, content string) (int, error) {
	depth := 0
	var parent interface{} // nil binds as NULL: a top-level comment
	if parentID != nil {
		var parentPost, parentDepth int
		err := s.db.QueryRow("SELECT post_id, depth FROM comments WHERE id = ?", *parentID).Scan(&parentPost, &parentDepth)
		if err == sql.ErrNoRows {
			return 0, apperrors.NotFoundError("Parent comment")
		}
		if err != nil {
			return 0, apperrors.DatabaseError(err)
		}
		if parentPost != postID {
			return 0, apperrors.ValidationError("parent comment belongs to a different post")
		}
		depth = parentDepth + 1
		parent = *parentID
	}
	res, err := s.db.Exec(
		"INSERT INTO comments (post_id, user_id, parent_id, depth, content, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		postID, userID, parent, depth, content, time.Now(),
	)
	if err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

const commentSelect = `
	SELECT c.id, c.post_id, c.user_id, c.parent_id, c.depth, c.content, c.created_at, u.username, c.deleted,
		(SELECT COUNT(*) FROM comment_likes WHERE comment_id = c.id AND is_like = 1),
		(SELECT COUNT(*) FROM comment_likes WHERE comment_id = c.id AND is_like = 0)
	FROM comments c JOIN users u ON c.user_id = u.id`

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanComment(row rowScanner) (*models.Comment, error) {
	var c models.Comment
	var parent sql.NullInt64
	if err := row.Scan(&c.ID, &c.PostID, &c.UserID, &parent, &c.Depth, &c.Content, &c.CreatedAt,
		&c.Author, &c.Deleted, &c.Likes, &c.Dislikes); err != nil {
		return nil, err
	}
	if parent.Valid {
		id := int(parent.Int64)
		c.ParentID = &id
	}
	return &c, nil
}

func (s *SQLiteStore) GetComment(id int) (*models.Comment, error) {
	c, err := scanComment(s.db.QueryRow(commentSelect+" WHERE c.id = ?", id))
	if err == sql.ErrNoRows {
		return nil, apperrors.NotFoundError("Comment")
	}
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	return c, nil
}

// GetCommentsByPostID returns every comment of a post as a flat list, oldest
// first; the reply structure is carried by ParentID and rebuilt by the client.
func (s *SQLiteStore) GetCommentsByPostID(postID int) ([]*models.Comment, error) {
	rows, err := s.db.Query(commentSelect+" WHERE c.post_id = ? ORDER BY c.created_at ASC, c.id ASC", postID)
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	defer rows.Close()
	var comments []*models.Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, apperrors.DatabaseError(err)
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	return comments, nil
}

// UpdateComment edits a comment's text. A tombstone can't be edited back to life.
func (s *SQLiteStore) UpdateComment(id, userID int, content string) error {
	res, err := s.db.Exec("UPDATE comments SET content = ? WHERE id = ? AND user_id = ? AND deleted = 0", content, id, userID)
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
	return s.deleteComment(id, &userID)
}

// AdminDeleteComment removes a comment regardless of ownership.
func (s *SQLiteStore) AdminDeleteComment(id int) error {
	return s.deleteComment(id, nil)
}

// deleteComment removes a comment, or - if other comments reply to it -
// replaces it with a tombstone so the replies keep their place in the thread.
// When the last reply under a tombstone goes, the tombstone goes with it. It
// all happens in one transaction, and ownership is checked before anything is
// touched: a rejected delete must not have side effects.
//
// ownerID nil means the caller may delete any comment (staff).
func (s *SQLiteStore) deleteComment(id int, ownerID *int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	defer func() { _ = tx.Rollback() }()

	var parent sql.NullInt64
	var owner int
	var deleted bool
	err = tx.QueryRow("SELECT parent_id, user_id, deleted FROM comments WHERE id = ?", id).Scan(&parent, &owner, &deleted)
	if err == sql.ErrNoRows {
		return apperrors.NotFoundError("Comment")
	}
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	if ownerID != nil && owner != *ownerID {
		return apperrors.PermissionError("delete comment")
	}
	if deleted {
		return apperrors.NotFoundError("Comment")
	}

	if _, err := tx.Exec("DELETE FROM comment_likes WHERE comment_id = ?", id); err != nil {
		return apperrors.DatabaseError(err)
	}

	var hasReplies bool
	if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM comments WHERE parent_id = ?)", id).Scan(&hasReplies); err != nil {
		return apperrors.DatabaseError(err)
	}
	if hasReplies {
		if _, err := tx.Exec("UPDATE comments SET deleted = 1, content = '' WHERE id = ?", id); err != nil {
			return apperrors.DatabaseError(err)
		}
		return commitComments(tx)
	}

	// A leaf: remove it, then any chain of now-childless tombstones above it.
	current := id
	for {
		if _, err := tx.Exec("DELETE FROM comments WHERE id = ?", current); err != nil {
			return apperrors.DatabaseError(err)
		}
		if !parent.Valid {
			break
		}
		parentID := int(parent.Int64)
		var parentDeleted, stillHasReplies bool
		var grandparent sql.NullInt64
		if err := tx.QueryRow("SELECT deleted, parent_id FROM comments WHERE id = ?", parentID).Scan(&parentDeleted, &grandparent); err != nil {
			return apperrors.DatabaseError(err)
		}
		if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM comments WHERE parent_id = ?)", parentID).Scan(&stillHasReplies); err != nil {
			return apperrors.DatabaseError(err)
		}
		if !parentDeleted || stillHasReplies {
			break
		}
		current, parent = parentID, grandparent
	}
	return commitComments(tx)
}

func commitComments(tx *sql.Tx) error {
	if err := tx.Commit(); err != nil {
		return apperrors.DatabaseError(err)
	}
	return nil
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

// ---- Direct message operations ----

// GetOrCreateConversation returns the conversation id between two users,
// creating it if needed. The pair is stored with the lower user id first so
// the (user_a_id, user_b_id) unique constraint catches both call orders.
func (s *SQLiteStore) GetOrCreateConversation(userA, userB int) (int, error) {
	a, b := userA, userB
	if a > b {
		a, b = b, a
	}
	if _, err := s.db.Exec("INSERT OR IGNORE INTO conversations (user_a_id, user_b_id) VALUES (?, ?)", a, b); err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	var id int
	if err := s.db.QueryRow("SELECT id FROM conversations WHERE user_a_id = ? AND user_b_id = ?", a, b).Scan(&id); err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	return id, nil
}

func (s *SQLiteStore) ListConversations(userID int) ([]*models.Conversation, error) {
	rows, err := s.db.Query(`
		SELECT
			c.id,
			CASE WHEN c.user_a_id = ? THEN ub.username ELSE ua.username END,
			CASE WHEN c.user_a_id = ? THEN ub.avatar_url ELSE ua.avatar_url END,
			COALESCE((SELECT content FROM messages WHERE conversation_id = c.id ORDER BY created_at DESC, id DESC LIMIT 1), ''),
			c.last_message_at,
			(SELECT COUNT(*) FROM messages WHERE conversation_id = c.id AND sender_id != ? AND read_at IS NULL)
		FROM conversations c
		JOIN users ua ON c.user_a_id = ua.id
		JOIN users ub ON c.user_b_id = ub.id
		WHERE c.user_a_id = ? OR c.user_b_id = ?
		ORDER BY c.last_message_at DESC, c.id DESC`, userID, userID, userID, userID, userID)
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	defer rows.Close()
	var convos []*models.Conversation
	for rows.Next() {
		var c models.Conversation
		if err := rows.Scan(&c.ID, &c.OtherUsername, &c.OtherAvatarURL, &c.LastMessage, &c.LastMessageAt, &c.UnreadCount); err != nil {
			return nil, apperrors.DatabaseError(err)
		}
		convos = append(convos, &c)
	}
	return convos, rows.Err()
}

func (s *SQLiteStore) GetMessages(conversationID, limit, offset int) ([]*models.Message, error) {
	rows, err := s.db.Query(`
		SELECT m.id, m.conversation_id, m.sender_id, u.username, m.content, m.created_at, m.read_at
		FROM messages m
		JOIN users u ON m.sender_id = u.id
		WHERE m.conversation_id = ?
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT ? OFFSET ?`, conversationID, limit, offset)
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	defer rows.Close()
	var messages []*models.Message
	for rows.Next() {
		var m models.Message
		var readAt sql.NullTime
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.Sender, &m.Content, &m.CreatedAt, &readAt); err != nil {
			return nil, apperrors.DatabaseError(err)
		}
		if readAt.Valid {
			t := readAt.Time
			m.ReadAt = &t
		}
		messages = append(messages, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	// Queried newest-first for a stable LIMIT/OFFSET window; reverse for
	// chronological (oldest-first) rendering.
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

func (s *SQLiteStore) CreateMessage(conversationID, senderID int, content string) (*models.Message, error) {
	res, err := s.db.Exec("INSERT INTO messages (conversation_id, sender_id, content) VALUES (?, ?, ?)", conversationID, senderID, content)
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.db.Exec("UPDATE conversations SET last_message_at = CURRENT_TIMESTAMP WHERE id = ?", conversationID); err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	var m models.Message
	err = s.db.QueryRow(`
		SELECT m.id, m.conversation_id, m.sender_id, u.username, m.content, m.created_at
		FROM messages m JOIN users u ON m.sender_id = u.id WHERE m.id = ?`, id).
		Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.Sender, &m.Content, &m.CreatedAt)
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	return &m, nil
}

func (s *SQLiteStore) MarkConversationRead(conversationID, userID int) error {
	_, err := s.db.Exec(
		"UPDATE messages SET read_at = CURRENT_TIMESTAMP WHERE conversation_id = ? AND sender_id != ? AND read_at IS NULL",
		conversationID, userID,
	)
	return err
}

func (s *SQLiteStore) CountUnreadMessages(userID int) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM messages m
		JOIN conversations c ON m.conversation_id = c.id
		WHERE (c.user_a_id = ? OR c.user_b_id = ?) AND m.sender_id != ? AND m.read_at IS NULL`,
		userID, userID, userID).Scan(&count)
	return count, err
}

// ---- Audit log operations ----

// CreateAuditEntry records a staff action. details must be a JSON object.
func (s *SQLiteStore) CreateAuditEntry(actorID int, action, targetType string, targetID int, details string) error {
	_, err := s.db.Exec(
		"INSERT INTO audit_log (actor_id, action, target_type, target_id, details, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		actorID, action, targetType, targetID, details, time.Now(),
	)
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	return nil
}

// ListAuditEntries returns recorded actions, newest first.
func (s *SQLiteStore) ListAuditEntries(limit, offset int) ([]*models.AuditEntry, error) {
	rows, err := s.db.Query(`
		SELECT a.id, a.actor_id, u.username, a.action, a.target_type, a.target_id, a.details, a.created_at
		FROM audit_log a JOIN users u ON u.id = a.actor_id
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	return scanAuditEntries(rows)
}

func (s *SQLiteStore) CountAuditEntries() (int, error) {
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM audit_log").Scan(&n); err != nil {
		return 0, apperrors.DatabaseError(err)
	}
	return n, nil
}

// scanAuditEntries is shared by both stores: the row shape is identical.
func scanAuditEntries(rows *sql.Rows) ([]*models.AuditEntry, error) {
	defer rows.Close()
	var entries []*models.AuditEntry
	for rows.Next() {
		var e models.AuditEntry
		var details string
		if err := rows.Scan(&e.ID, &e.ActorID, &e.Actor, &e.Action, &e.TargetType, &e.TargetID, &details, &e.CreatedAt); err != nil {
			return nil, apperrors.DatabaseError(err)
		}
		e.Details = json.RawMessage(details)
		entries = append(entries, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	return entries, nil
}

// ---- Block operations ----

// BlockUser records that blockerID has blocked blockedID. Blocking someone
// already blocked is a no-op, so the endpoint is safe to retry.
//
// ON CONFLICT DO NOTHING rather than INSERT OR IGNORE: the latter would also
// swallow a CHECK violation, silently "succeeding" at blocking yourself where
// PostgreSQL rejects it.
func (s *SQLiteStore) BlockUser(blockerID, blockedID int) error {
	_, err := s.db.Exec("INSERT INTO user_blocks (blocker_id, blocked_id) VALUES (?, ?) ON CONFLICT DO NOTHING", blockerID, blockedID)
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	return nil
}

func (s *SQLiteStore) UnblockUser(blockerID, blockedID int) error {
	_, err := s.db.Exec("DELETE FROM user_blocks WHERE blocker_id = ? AND blocked_id = ?", blockerID, blockedID)
	if err != nil {
		return apperrors.DatabaseError(err)
	}
	return nil
}

// IsBlocked reports whether blockerID has blocked blockedID. It is directional:
// A blocking B says nothing about B blocking A.
func (s *SQLiteStore) IsBlocked(blockerID, blockedID int) (bool, error) {
	var blocked bool
	err := s.db.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM user_blocks WHERE blocker_id = ? AND blocked_id = ?)",
		blockerID, blockedID,
	).Scan(&blocked)
	if err != nil {
		return false, apperrors.DatabaseError(err)
	}
	return blocked, nil
}

// ListBlockedUsers returns the users blockerID has blocked, most recent first.
func (s *SQLiteStore) ListBlockedUsers(blockerID int) ([]*models.UserSummary, error) {
	rows, err := s.db.Query(`
		SELECT u.id, u.username, u.avatar_url
		FROM user_blocks b JOIN users u ON u.id = b.blocked_id
		WHERE b.blocker_id = ?
		ORDER BY b.created_at DESC, u.id DESC`, blockerID)
	if err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	defer rows.Close()
	var users []*models.UserSummary
	for rows.Next() {
		var u models.UserSummary
		if err := rows.Scan(&u.ID, &u.Username, &u.AvatarURL); err != nil {
			return nil, apperrors.DatabaseError(err)
		}
		users = append(users, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.DatabaseError(err)
	}
	return users, nil
}

// ---- Utility ----

func (s *SQLiteStore) Close() error  { return s.db.Close() }
func (s *SQLiteStore) Health() error { return s.db.Ping() }
