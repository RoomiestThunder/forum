package models

import (
	"encoding/json"
	"time"
)

type User struct {
	ID            int       `json:"id"`
	Email         string    `json:"email"`
	Username      string    `json:"username"`
	Password      string    `json:"-"`
	AvatarURL     string    `json:"avatar_url"`
	Bio           string    `json:"bio"`
	IsAdmin       bool      `json:"is_admin"`
	IsModerator   bool      `json:"is_moderator"`
	Banned        bool      `json:"banned"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
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

// UserSummary is the minimal public shape used for @mention autocomplete
// and similar lightweight lookups where a full User/PublicProfile would
// expose more than needed.
type UserSummary struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
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
	Draft        bool       `json:"draft"`
}

type Comment struct {
	ID     int `json:"id"`
	PostID int `json:"post_id"`
	UserID int `json:"user_id"`
	// ParentID is the comment this one replies to; nil for a top-level comment.
	ParentID *int `json:"parent_id"`
	// Depth is the nesting level: 0 for top-level, parent's depth + 1 for a reply.
	Depth     int       `json:"depth"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	Author    string    `json:"author"`
	Likes     int       `json:"likes"`
	Dislikes  int       `json:"dislikes"`
	// Deleted marks a tombstone: a deleted comment that is kept because it has
	// replies. Its content and author are cleared before it is served.
	Deleted bool `json:"deleted"`
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

// Conversation is a 1:1 direct-message thread, from one participant's
// point of view - "other" always refers to whoever isn't the caller.
type Conversation struct {
	ID             int       `json:"id"`
	OtherUsername  string    `json:"other_username"`
	OtherAvatarURL string    `json:"other_avatar_url"`
	LastMessage    string    `json:"last_message"`
	LastMessageAt  time.Time `json:"last_message_at"`
	UnreadCount    int       `json:"unread_count"`
}

type Message struct {
	ID             int        `json:"id"`
	ConversationID int        `json:"conversation_id"`
	SenderID       int        `json:"sender_id"`
	Sender         string     `json:"sender"`
	Content        string     `json:"content"`
	CreatedAt      time.Time  `json:"created_at"`
	ReadAt         *time.Time `json:"read_at,omitempty"`
}

type PaginatedPosts struct {
	Posts      []*Post `json:"posts"`
	Page       int     `json:"page"`
	TotalPages int     `json:"total_pages"`
	TotalCount int     `json:"total_count"`
}

// AuditEntry is one recorded staff action. Details is a JSON object holding a
// snapshot of whatever was acted on, so the entry stays meaningful after the
// target is deleted.
type AuditEntry struct {
	ID         int             `json:"id"`
	ActorID    int             `json:"actor_id"`
	Actor      string          `json:"actor"`
	Action     string          `json:"action"`
	TargetType string          `json:"target_type"`
	TargetID   int             `json:"target_id"`
	Details    json.RawMessage `json:"details"`
	CreatedAt  time.Time       `json:"created_at"`
}
