package api

import (
	"net/http"

	"forum/internal/auth"
	"forum/internal/cache"
	"forum/internal/database"
	"forum/internal/metrics"
	"forum/internal/storage"
	ws "forum/internal/websocket"
)

type Server struct {
	store   database.Store
	jwt     *auth.Manager
	cache   *cache.Cache
	storage *storage.Client
	hub     *ws.Hub
}

func NewServer(
	store database.Store,
	jwt *auth.Manager,
	redisCache *cache.Cache,
	storageClient *storage.Client,
	hub *ws.Hub,
) *Server {
	return &Server{
		store:   store,
		jwt:     jwt,
		cache:   redisCache,
		storage: storageClient,
		hub:     hub,
	}
}

// Mount registers all /api/v1/* routes onto the given mux.
func (s *Server) Mount(mux *http.ServeMux) {
	mux.Handle("/api/v1/", metrics.Middleware(http.HandlerFunc(s.route)))
	mux.Handle("/api/v1/ws", http.HandlerFunc(s.handleWS))
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	// Apply Redis rate limiting to all API routes
	if s.cache != nil {
		ip := r.RemoteAddr
		allowed, err := s.cache.AllowRequest(r.Context(), ip)
		if err == nil && !allowed {
			jsonError(w, "too many requests", http.StatusTooManyRequests)
			return
		}
	}

	path := r.URL.Path
	switch {
	// Auth
	case path == "/api/v1/auth/register" && r.Method == http.MethodPost:
		s.register(w, r)
	case path == "/api/v1/auth/login" && r.Method == http.MethodPost:
		s.login(w, r)
	case path == "/api/v1/auth/refresh" && r.Method == http.MethodPost:
		s.refreshToken(w, r)
	case path == "/api/v1/auth/logout" && r.Method == http.MethodPost:
		s.requireAuth(s.logout)(w, r)

	// Posts
	case path == "/api/v1/posts" && r.Method == http.MethodGet:
		s.listPosts(w, r)
	case path == "/api/v1/posts" && r.Method == http.MethodPost:
		s.requireAuth(s.createPost)(w, r)
	case matchPath(path, "/api/v1/posts/") && r.Method == http.MethodGet:
		s.getPost(w, r)
	case matchPath(path, "/api/v1/posts/") && r.Method == http.MethodPut:
		s.requireAuth(s.updatePost)(w, r)
	case matchPath(path, "/api/v1/posts/") && r.Method == http.MethodDelete:
		s.requireAuth(s.deletePost)(w, r)

	// Comments
	case matchPath(path, "/api/v1/posts/") && r.Method == http.MethodPost:
		s.requireAuth(s.createComment)(w, r)

	// Likes
	case path == "/api/v1/like/post" && r.Method == http.MethodPost:
		s.requireAuth(s.likePost)(w, r)
	case path == "/api/v1/like/comment" && r.Method == http.MethodPost:
		s.requireAuth(s.likeComment)(w, r)

	// Search
	case path == "/api/v1/search" && r.Method == http.MethodGet:
		s.search(w, r)

	// Upload
	case path == "/api/v1/upload" && r.Method == http.MethodPost:
		s.requireAuth(s.upload)(w, r)

	// Categories
	case path == "/api/v1/categories" && r.Method == http.MethodGet:
		s.listCategories(w, r)

	// Health
	case path == "/api/v1/health" && r.Method == http.MethodGet:
		s.health(w, r)

	default:
		jsonError(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}

// matchPath returns true if path starts with the given prefix (and has more content after it).
func matchPath(path, prefix string) bool {
	return len(path) > len(prefix) && path[:len(prefix)] == prefix
}
