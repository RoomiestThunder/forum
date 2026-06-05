package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) listPosts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize := 10
	offset := (page - 1) * pageSize

	var cacheKey string
	userID := userIDFromCtx(r)

	filter := q.Get("filter")
	category := q.Get("category")

	// Try cache for default listing
	if filter == "" && category == "" && s.cache != nil {
		cacheKey = fmt.Sprintf("page=%d", page)
		if posts, err := s.cache.GetPosts(r.Context(), cacheKey); err == nil && posts != nil {
			jsonResponse(w, posts, http.StatusOK)
			return
		}
	}

	var (
		posts interface{}
		err   error
	)

	switch {
	case filter == "myposts" && userID != 0:
		posts, err = s.store.GetPostsByUser(userID, pageSize, offset)
	case filter == "liked" && userID != 0:
		posts, err = s.store.GetLikedPostsByUser(userID, pageSize, offset)
	case category != "":
		catID, _ := strconv.Atoi(category)
		posts, err = s.store.GetPostsByCategory(catID, pageSize, offset)
	default:
		p, e := s.store.GetPosts(pageSize, offset)
		posts, err = p, e
		// Cache the result
		if e == nil && cacheKey != "" && s.cache != nil {
			_ = s.cache.SetPosts(context.Background(), cacheKey, p)
		}
	}

	if err != nil {
		jsonError(w, "failed to load posts", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, posts, http.StatusOK)
}

func (s *Server) getPost(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/v1/posts/")
	if err != nil {
		jsonError(w, "invalid post id", http.StatusBadRequest)
		return
	}
	post, err := s.store.GetPost(id)
	if err != nil {
		jsonError(w, "post not found", http.StatusNotFound)
		return
	}
	comments, _ := s.store.GetCommentsByPostID(id)
	jsonResponse(w, map[string]interface{}{"post": post, "comments": comments}, http.StatusOK)
}

func (s *Server) createPost(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title       string `json:"title"`
		Content     string `json:"content"`
		CategoryIDs []int  `json:"category_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	body.Content = strings.TrimSpace(body.Content)
	if body.Title == "" || body.Content == "" || len(body.CategoryIDs) == 0 {
		jsonError(w, "title, content, and at least one category_id required", http.StatusBadRequest)
		return
	}
	if len([]rune(body.Title)) > 20 {
		jsonError(w, "title must be 20 characters or less", http.StatusBadRequest)
		return
	}

	userID := userIDFromCtx(r)
	postID, err := s.store.CreatePost(userID, body.Title, body.Content, body.CategoryIDs)
	if err != nil {
		jsonError(w, "failed to create post", http.StatusInternalServerError)
		return
	}

	// Invalidate posts cache
	if s.cache != nil {
		_ = s.cache.InvalidatePosts(r.Context())
	}

	jsonResponse(w, map[string]int{"id": postID}, http.StatusCreated)
}

func (s *Server) updatePost(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/v1/posts/")
	if err != nil {
		jsonError(w, "invalid post id", http.StatusBadRequest)
		return
	}
	var body struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	userID := userIDFromCtx(r)
	if err := s.store.UpdatePost(id, userID, body.Title, body.Content); err != nil {
		status := http.StatusInternalServerError
		if isPermissionErr(err) {
			status = http.StatusForbidden
		}
		jsonError(w, err.Error(), status)
		return
	}
	if s.cache != nil {
		_ = s.cache.InvalidatePosts(r.Context())
	}
	jsonResponse(w, map[string]string{"status": "updated"}, http.StatusOK)
}

func (s *Server) deletePost(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath(r.URL.Path, "/api/v1/posts/")
	if err != nil {
		jsonError(w, "invalid post id", http.StatusBadRequest)
		return
	}
	userID := userIDFromCtx(r)
	if err := s.store.DeletePost(id, userID); err != nil {
		status := http.StatusInternalServerError
		if isPermissionErr(err) {
			status = http.StatusForbidden
		}
		jsonError(w, err.Error(), status)
		return
	}
	if s.cache != nil {
		_ = s.cache.InvalidatePosts(r.Context())
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createComment(w http.ResponseWriter, r *http.Request) {
	postID, err := parseIDFromPath(r.URL.Path, "/api/v1/posts/")
	if err != nil {
		jsonError(w, "invalid post id", http.StatusBadRequest)
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.Content == "" || len(body.Content) > 500 {
		jsonError(w, "content must be 1-500 chars", http.StatusBadRequest)
		return
	}
	userID := userIDFromCtx(r)
	commentID, err := s.store.CreateComment(postID, userID, body.Content)
	if err != nil {
		jsonError(w, "failed to create comment", http.StatusInternalServerError)
		return
	}

	// Notify the post author about new comment (best effort)
	if s.hub != nil {
		post, err := s.store.GetPost(postID)
		if err == nil && post.UserID != userID {
			s.hub.Notify(post.UserID, "new_comment", map[string]interface{}{
				"post_id":    postID,
				"comment_id": commentID,
			})
		}
	}

	jsonResponse(w, map[string]int{"id": commentID}, http.StatusCreated)
}

func (s *Server) likePost(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PostID int  `json:"post_id"`
		IsLike bool `json:"is_like"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	userID := userIDFromCtx(r)
	if err := s.store.TogglePostLike(body.PostID, userID, body.IsLike); err != nil {
		jsonError(w, "like failed", http.StatusInternalServerError)
		return
	}
	// Invalidate cache since like counts changed
	if s.cache != nil {
		_ = s.cache.InvalidatePosts(r.Context())
	}
	// Notify post author
	if s.hub != nil {
		post, err := s.store.GetPost(body.PostID)
		if err == nil && post.UserID != userID {
			s.hub.Notify(post.UserID, "new_like", map[string]interface{}{
				"post_id": body.PostID,
				"is_like": body.IsLike,
			})
		}
	}
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (s *Server) likeComment(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CommentID int  `json:"comment_id"`
		IsLike    bool `json:"is_like"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	userID := userIDFromCtx(r)
	if err := s.store.ToggleCommentLike(body.CommentID, userID, body.IsLike); err != nil {
		jsonError(w, "like failed", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		jsonError(w, "q parameter required", http.StatusBadRequest)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * 10

	posts, total, err := s.store.SearchPosts(q, 10, offset)
	if err != nil {
		jsonError(w, "search failed", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]interface{}{
		"posts": posts,
		"total": total,
		"page":  page,
	}, http.StatusOK)
}

func (s *Server) listCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := s.store.GetCategories()
	if err != nil {
		jsonError(w, "failed to load categories", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, cats, http.StatusOK)
}

func parseIDFromPath(path, prefix string) (int, error) {
	trimmed := strings.TrimPrefix(path, prefix)
	// Handle nested paths like /api/v1/posts/5/comments
	parts := strings.Split(trimmed, "/")
	return strconv.Atoi(parts[0])
}
