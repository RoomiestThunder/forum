package handlers

import "time"

type User struct {
	ID       int
	Email    string
	Username string
	Password string
}

type Category struct {
	ID   int
	Name string
}

type Post struct {
	ID           int
	UserID       int
	Title        string
	Content      string
	CreatedAt    string
	Author       string
	Likes        int
	Dislikes     int
	Categories   []Category
	CommentCount int
}

type Comment struct {
	ID        int
	PostID    int
	UserID    int
	Content   string
	CreatedAt string
	Author    string
	Likes     int
	Dislikes  int
}

type Session struct {
	ID      int
	UserID  int
	UUID    string
	Expires time.Time
}
