package handlers

// Re-export model types for template compatibility.
// Templates use handlers.User, handlers.Post, etc.

import "forum/internal/models"

type User = models.User
type Post = models.Post
type Comment = models.Comment
type Category = models.Category
type Session = models.Session
