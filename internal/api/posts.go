package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"forum/internal/models"
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
	author := q.Get("author")
	tag := q.Get("tag")

	// Try cache for default listing
	if filter == "" && category == "" && author == "" && tag == "" && s.cache != nil {
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
	case filter == "drafts" && userID != 0:
		posts, err = s.store.GetDraftPostsByUser(userID, pageSize, offset)
	case filter == "liked" && userID != 0:
		posts, err = s.store.GetLikedPostsByUser(userID, pageSize, offset)
	case filter == "saved" && userID != 0:
		posts, err = s.store.GetBookmarkedPostsByUser(userID, pageSize, offset)
	case author != "":
		var u *models.User
		u, err = s.store.GetUserByUsername(author)
		if err == nil {
			posts, err = s.store.GetPostsByUser(u.ID, pageSize, offset)
		} else {
			posts, err = []*models.Post{}, nil
		}
	case tag != "":
		posts, err = s.store.GetPostsByTag(tag, pageSize, offset)
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
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid post id", http.StatusBadRequest)
		return
	}
	post, err := s.store.GetPost(id)
	if err != nil {
		jsonError(w, "post not found", http.StatusNotFound)
		return
	}
	if post.Draft && post.UserID != userIDFromCtx(r) {
		jsonError(w, "post not found", http.StatusNotFound)
		return
	}
	comments, err := s.store.GetCommentsByPostID(id)
	if err != nil {
		s.storeError(w, r, err, "load comments")
		return
	}

	resp := map[string]interface{}{"post": post, "comments": redactDeleted(comments)}
	if userID := userIDFromCtx(r); userID != 0 {
		bookmarked, _ := s.store.IsBookmarked(id, userID)
		resp["is_bookmarked"] = bookmarked
	}
	jsonResponse(w, resp, http.StatusOK)
}

func (s *Server) createPost(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title       string   `json:"title"`
		Content     string   `json:"content"`
		CategoryIDs []int    `json:"category_ids"`
		ImageURL    string   `json:"image_url"`
		Tags        []string `json:"tags"`
		Draft       bool     `json:"draft"`
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
	if len([]rune(body.Title)) > 100 {
		jsonError(w, "title must be 100 characters or less", http.StatusBadRequest)
		return
	}
	if len(body.Tags) > 5 {
		jsonError(w, "at most 5 tags allowed", http.StatusBadRequest)
		return
	}

	userID := userIDFromCtx(r)
	postID, err := s.store.CreatePost(userID, body.Title, body.Content, body.CategoryIDs, body.ImageURL, body.Draft)
	if err != nil {
		jsonError(w, "failed to create post", http.StatusInternalServerError)
		return
	}
	if len(body.Tags) > 0 {
		_ = s.store.SetPostTags(postID, body.Tags)
	}

	// Invalidate posts cache
	if s.cache != nil {
		_ = s.cache.InvalidatePosts(r.Context())
	}

	jsonResponse(w, map[string]int{"id": postID}, http.StatusCreated)
}

// publishPost flips a draft to published. Owner-only.
func (s *Server) publishPost(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid post id", http.StatusBadRequest)
		return
	}
	userID := userIDFromCtx(r)
	if err := s.store.SetPostDraft(id, userID, false); err != nil {
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
	jsonResponse(w, map[string]string{"status": "published"}, http.StatusOK)
}

func (s *Server) updatePost(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
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
	body.Title = strings.TrimSpace(body.Title)
	body.Content = strings.TrimSpace(body.Content)
	if body.Title == "" || body.Content == "" {
		jsonError(w, "title and content required", http.StatusBadRequest)
		return
	}
	if len([]rune(body.Title)) > 100 {
		jsonError(w, "title must be 100 characters or less", http.StatusBadRequest)
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
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
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

const (
	maxCommentLength = 500
	// maxCommentDepth is how many levels of replies may sit below a top-level
	// comment. Past this a thread is too narrow to read, and the client has no
	// room left to indent it.
	maxCommentDepth = 5
)

func (s *Server) createComment(w http.ResponseWriter, r *http.Request) {
	postID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonError(w, "invalid post id", http.StatusBadRequest)
		return
	}
	var body struct {
		Content  string `json:"content"`
		ParentID *int   `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	// Count characters, not bytes: a byte limit would cap Cyrillic or CJK text
	// at a fraction of what the same limit allows in English.
	body.Content = strings.TrimSpace(body.Content)
	if body.Content == "" || utf8.RuneCountInString(body.Content) > maxCommentLength {
		jsonError(w, fmt.Sprintf("content must be 1-%d characters", maxCommentLength), http.StatusBadRequest)
		return
	}
	userID := userIDFromCtx(r)

	post, err := s.store.GetPost(postID)
	if err != nil || (post.Draft && post.UserID != userID) {
		// A draft answers exactly like a missing post, so it isn't revealed.
		jsonError(w, "post not found", http.StatusNotFound)
		return
	}

	var parent *models.Comment
	if body.ParentID != nil {
		parent, err = s.store.GetComment(*body.ParentID)
		if err != nil || parent.PostID != postID {
			jsonError(w, "parent comment not found", http.StatusNotFound)
			return
		}
		if parent.Deleted {
			jsonError(w, "you can't reply to a deleted comment", http.StatusBadRequest)
			return
		}
		if parent.Depth >= maxCommentDepth {
			jsonError(w, "this thread is nested too deeply to reply to", http.StatusBadRequest)
			return
		}
	}

	commentID, err := s.store.CreateComment(postID, userID, body.ParentID, body.Content)
	if err != nil {
		s.storeError(w, r, err, "create comment")
		return
	}

	s.notifyAboutComment(r, post, parent, commentID, body.Content)
	jsonResponse(w, map[string]int{"id": commentID}, http.StatusCreated)
}

// notifyAboutComment tells the people a new comment concerns. Each person
// gets at most one of the reply / post-comment notifications: the author of
// the comment being replied to hears "new_reply", and the post's author hears
// "new_comment" only if that isn't the same person. Nobody is notified about
// their own comment.
//
// @mentions are a separate signal and are not deduplicated against those two,
// even if the mentioned user is the post author.
func (s *Server) notifyAboutComment(r *http.Request, post *models.Post, parent *models.Comment, commentID int, content string) {
	actorID := userIDFromCtx(r)
	payload := func() map[string]interface{} {
		p := map[string]interface{}{
			"post_id":    post.ID,
			"post_title": post.Title,
			"comment_id": commentID,
			"actor":      usernameFromCtx(r),
		}
		if parent != nil {
			p["parent_id"] = parent.ID
		}
		return p
	}

	notified := map[int]bool{actorID: true}
	if parent != nil && !notified[parent.UserID] {
		notified[parent.UserID] = true
		s.notify(parent.UserID, actorID, "new_reply", payload())
	}
	if !notified[post.UserID] {
		notified[post.UserID] = true
		s.notify(post.UserID, actorID, "new_comment", payload())
	}

	for _, username := range extractMentions(content) {
		mentioned, err := s.store.GetUserByUsername(username)
		if err != nil || mentioned.ID == actorID {
			continue
		}
		s.notify(mentioned.ID, actorID, "new_mention", payload())
	}
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
	// Notify post author (only for likes, not dislikes - nobody wants a ping for that).
	if post, err := s.store.GetPost(body.PostID); err == nil && post.UserID != userID && body.IsLike {
		s.notify(post.UserID, userID, "new_like", map[string]interface{}{
			"post_id":    body.PostID,
			"post_title": post.Title,
			"actor":      usernameFromCtx(r),
		})
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
