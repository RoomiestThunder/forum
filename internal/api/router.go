package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"forum/internal/auth"
	"forum/internal/cache"
	"forum/internal/database"
	"forum/internal/metrics"
	appmw "forum/internal/middleware"
	"forum/internal/storage"
	ws "forum/internal/websocket"
)

type Server struct {
	store   database.Store
	jwt     *auth.Manager
	cache   *cache.Cache
	storage *storage.Client
	hub     *ws.Hub

	// rateLimiter is used only when Redis is unavailable, so basic abuse
	// protection still holds even in cache-degraded mode.
	rateLimiter *appmw.RateLimiter
}

func NewServer(
	store database.Store,
	jwt *auth.Manager,
	redisCache *cache.Cache,
	storageClient *storage.Client,
	hub *ws.Hub,
) *Server {
	s := &Server{
		store:   store,
		jwt:     jwt,
		cache:   redisCache,
		storage: storageClient,
		hub:     hub,
	}
	if redisCache == nil {
		s.rateLimiter = appmw.NewRateLimiter(1, 60) // ~60 req/min per IP, matches the Redis limit
	}
	return s
}

// Mount registers /api/v1/* (including the websocket upgrade) onto root.
func (s *Server) Mount(root chi.Router) {
	// The websocket upgrade is a long-lived connection, not a request/response
	// cycle, so it stays out of the HTTP metrics histogram and rate limiter.
	root.Get("/api/v1/ws", s.handleWS)

	root.Route("/api/v1", func(r chi.Router) {
		r.Use(metrics.Middleware)
		r.Use(s.rateLimit)
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			jsonError(w, "not found", http.StatusNotFound)
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		})

		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", s.register)
			r.Post("/login", s.login)
			r.Post("/refresh", s.refreshToken)
			r.With(s.requireAuth).Post("/logout", s.logout)
		})

		r.With(s.requireAuth).Get("/me", s.me)
		r.With(s.requireAuth).Put("/me/bio", s.updateBio)
		r.With(s.requireAuth).Post("/me/avatar", s.uploadAvatar)
		r.Get("/users/{username}", s.profile)

		r.Route("/posts", func(r chi.Router) {
			r.With(s.optionalAuth).Get("/", s.listPosts)
			r.With(s.requireAuth).Post("/", s.createPost)
			r.With(s.optionalAuth).Get("/{id}", s.getPost)
			r.With(s.requireAuth).Put("/{id}", s.updatePost)
			r.With(s.requireAuth).Delete("/{id}", s.deletePost)
			r.With(s.requireAuth).Post("/{id}", s.createComment)
		})

		r.Route("/comments", func(r chi.Router) {
			r.With(s.requireAuth).Put("/{id}", s.updateComment)
			r.With(s.requireAuth).Delete("/{id}", s.deleteComment)
		})

		r.Route("/like", func(r chi.Router) {
			r.With(s.requireAuth).Post("/post", s.likePost)
			r.With(s.requireAuth).Post("/comment", s.likeComment)
		})

		r.With(s.requireAuth).Post("/bookmarks", s.toggleBookmark)
		r.Get("/tags", s.searchTags)

		r.Get("/search", s.search)
		r.With(s.requireAuth).Post("/upload", s.upload)
		r.Get("/categories", s.listCategories)
		r.Get("/health", s.health)
	})
}

// rateLimit applies Redis-backed rate limiting when available, falling back
// to an in-memory token bucket per IP otherwise.
func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		switch {
		case s.cache != nil:
			if allowed, err := s.cache.AllowRequest(r.Context(), ip); err == nil && !allowed {
				jsonError(w, "too many requests", http.StatusTooManyRequests)
				return
			}
		case s.rateLimiter != nil:
			if !s.rateLimiter.Allow(ip) {
				jsonError(w, "too many requests", http.StatusTooManyRequests)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}
