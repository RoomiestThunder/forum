package models

import "time"

type User struct {
	ID          int       `json:"id"`
	Email       string    `json:"email"`
	Username    string    `json:"username"`
	Password    string    `json:"-"`
	AvatarURL   string    `json:"avatar_url"`
	Bio         string    `json:"bio"`
	IsAdmin     bool      `json:"is_admin"`
	IsModerator bool      `json:"is_moderator"`
	Banned      bool      `json:"banned"`
	CreatedAt   time.Time `json:"created_at"`
}

// PublicProfile is what GET /api/v1/users/{username} returns - a User
// without the fields only the owner (or nobody) should see.
type PublicProfile struct {
	Username  string    `json:"username"`
	AvatarURL string    `json:"avatar_url"`
	Bio       string    `json:"bio"`
	CreatedAt time.Time `json:"created_at"`
	PostCount int       `json:"post_count"`
}

type Category struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Tag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Post struct {
	ID           int        `json:"id"`
	UserID       int        `json:"user_id"`
	Title        string     `json:"title"`
	Content      string     `json:"content"`
	CreatedAt    time.Time  `json:"created_at"`
	Author       string     `json:"author"`
	Likes        int        `json:"likes"`
	Dislikes     int        `json:"dislikes"`
	Categories   []Category `json:"categories"`
	Tags         []Tag      `json:"tags"`
	CommentCount int        `json:"comment_count"`
	ImageURL     string     `json:"image_url,omitempty"`
	Pinned       bool       `json:"pinned"`
}

type Comment struct {
	ID        int       `json:"id"`
	PostID    int       `json:"post_id"`
	UserID    int       `json:"user_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	Author    string    `json:"author"`
	Likes     int       `json:"likes"`
	Dislikes  int       `json:"dislikes"`
}

type Session struct {
	ID      int       `json:"id"`
	UserID  int       `json:"user_id"`
	UUID    string    `json:"uuid"`
	Expires time.Time `json:"expires"`
}

type RefreshToken struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Upload struct {
	ID          int       `json:"id"`
	UserID      int       `json:"user_id"`
	Filename    string    `json:"filename"`
	ObjectKey   string    `json:"object_key"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	URL         string    `json:"url"`
	CreatedAt   time.Time `json:"created_at"`
}

// Notification is the ephemeral envelope pushed over the websocket hub.
type Notification struct {
	Type    string      `json:"type"`
	UserID  int         `json:"user_id"`
	Payload interface{} `json:"payload"`
}

// NotificationRecord is a persisted notification row. Payload is stored as
// raw JSON text so each notification type can carry its own shape without
// schema changes.
type NotificationRecord struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Type      string    `json:"type"`
	Payload   string    `json:"-"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

// Report is a user-filed flag against a post or comment, queued for
// moderator/admin review.
type Report struct {
	ID               int       `json:"id"`
	ReporterID       int       `json:"reporter_id"`
	ReporterUsername string    `json:"reporter_username"`
	TargetType       string    `json:"target_type"` // "post" or "comment"
	TargetID         int       `json:"target_id"`
	Reason           string    `json:"reason"`
	Status           string    `json:"status"` // "open" or "resolved"
	CreatedAt        time.Time `json:"created_at"`
}

type PaginatedPosts struct {
	Posts      []*Post `json:"posts"`
	Page       int     `json:"page"`
	TotalPages int     `json:"total_pages"`
	TotalCount int     `json:"total_count"`
}
