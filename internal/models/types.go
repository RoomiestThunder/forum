package models

import "time"

type User struct {
	ID        int    `json:"id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	Password  string `json:"-"`
	AvatarURL string `json:"avatar_url"`
}

type Category struct {
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
	CommentCount int        `json:"comment_count"`
	ImageURL     string     `json:"image_url,omitempty"`
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

type Notification struct {
	Type    string      `json:"type"`
	UserID  int         `json:"user_id"`
	Payload interface{} `json:"payload"`
}

type PaginatedPosts struct {
	Posts      []*Post `json:"posts"`
	Page       int     `json:"page"`
	TotalPages int     `json:"total_pages"`
	TotalCount int     `json:"total_count"`
}
