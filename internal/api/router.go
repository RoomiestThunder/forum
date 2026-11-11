package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"golang.org/x/oauth2"
	githuboauth "golang.org/x/oauth2/github"

	"forum/internal/auth"
	"forum/internal/cache"
	"forum/internal/database"
	"forum/internal/mail"
	"forum/internal/metrics"
	appmw "forum/internal/middleware"
	"forum/internal/storage"
	ws "forum/internal/websocket"
)

// Options carries the pieces of Server that are optional / config-derived,
// so adding one doesn't mean touching every existing NewServer call site.
type Options struct {
	// BaseURL is used to build links in emails and the OAuth redirect URI.
	BaseURL string
	// GitHubClientID/Secret enable "Sign in with GitHub" when both are set.
	GitHubClientID     string
	GitHubClientSecret string
	// TrustProxy makes rate limiting identify clients by X-Forwarded-For /
	// X-Real-IP. Enable it only behind a reverse proxy that sets them;
	// otherwise clients could spoof their way past the limits.
	TrustProxy bool
}

type Server struct {
	store   database.Store
	jwt     *auth.Manager
	cache   *cache.Cache
	storage *storage.Client
	hub     *ws.Hub
	mailer  *mail.Mailer
	baseURL string

	// githubOAuth is nil when GitHub OAuth isn't configured; routes and the
	// providers endpoint check for that instead of a separate bool.
	githubOAuth *oauth2.Config

	// limiters are the in-process rate limiters, one per policy. They serve as
	// the limiter when Redis isn't configured and as the fallback when it is
	// unreachable, so limits keep holding in cache-degraded mode.
	limiters   map[string]*appmw.RateLimiter
	trustProxy bool
}

func NewServer(
	store database.Store,
	jwt *auth.Manager,
	redisCache *cache.Cache,
	storageClient *storage.Client,
	hub *ws.Hub,
	mailer *mail.Mailer,
	opts Options,
) *Server {
	s := &Server{
		store:   store,
		jwt:     jwt,
		cache:   redisCache,
		storage: storageClient,
		hub:     hub,
		mailer:  mailer,
		baseURL: opts.BaseURL,

		limiters:   newLocalLimiters(),
		trustProxy: opts.TrustProxy,
	}
	if opts.GitHubClientID != "" && opts.GitHubClientSecret != "" {
		s.githubOAuth = &oauth2.Config{
			ClientID:     opts.GitHubClientID,
			ClientSecret: opts.GitHubClientSecret,
			Endpoint:     githuboauth.Endpoint,
			RedirectURL:  opts.BaseURL + "/api/v1/auth/github/callback",
			Scopes:       []string{"read:user", "user:email"},
		}
	}
	return s
}

// Mount registers /api/v1/* (including the websocket upgrade) onto root.
func (s *Server) Mount(root chi.Router) {
	// The websocket upgrade is a long-lived connection, not a request/response
	// cycle, so it stays out of the HTTP metrics histogram and the global
	// limiter; the upgrade itself still gets its own per-IP limit.
	root.With(s.limitByIP(limitSocket)).Get("/api/v1/ws", s.handleWS)

	root.Route("/api/v1", func(r chi.Router) {
		r.Use(metrics.Middleware)
		r.Use(s.limitByIP(limitGlobal))
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			jsonError(w, "not found", http.StatusNotFound)
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		})

		r.Route("/auth", func(r chi.Router) {
			r.With(s.limitByIP(limitAuth)).Post("/register", s.register)
			r.With(s.limitByIP(limitAuth)).Post("/login", s.login)
			r.Post("/refresh", s.refreshToken)
			r.With(s.requireAuth).Post("/logout", s.logout)

			r.Post("/verify-email", s.verifyEmail)
			r.With(s.requireAuth, s.limitByIP(limitEmail)).Post("/resend-verification", s.resendVerification)
			r.With(s.limitByIP(limitEmail)).Post("/forgot-password", s.forgotPassword)
			r.With(s.limitByIP(limitAuth)).Post("/reset-password", s.resetPassword)

			r.Get("/providers", s.authProviders)
			r.Get("/github", s.githubLogin)
			r.Get("/github/callback", s.githubCallback)
		})

		r.With(s.requireAuth).Get("/me", s.me)
		r.With(s.requireAuth).Put("/me/bio", s.updateBio)
		r.With(s.requireAuth).Post("/me/avatar", s.uploadAvatar)
		r.Get("/users/search", s.searchUsers)
		r.Get("/users/{username}", s.profile)

		r.Route("/posts", func(r chi.Router) {
			r.With(s.optionalAuth).Get("/", s.listPosts)
			r.With(s.requireAuth, s.limitByUser(limitContent)).Post("/", s.createPost)
			r.With(s.optionalAuth).Get("/{id}", s.getPost)
			r.With(s.requireAuth).Put("/{id}", s.updatePost)
			r.With(s.requireAuth).Delete("/{id}", s.deletePost)
			r.With(s.requireAuth).Post("/{id}/publish", s.publishPost)
			r.With(s.requireAuth, s.limitByUser(limitContent)).Post("/{id}", s.createComment)
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
		r.With(s.requireAuth).Post("/reports", s.createReport)

		r.Route("/admin", func(r chi.Router) {
			r.Use(s.requireAuth, s.requireStaff)

			r.Get("/reports", s.listReports)
			r.Post("/reports/{id}/resolve", s.resolveReport)
			r.Post("/posts/{id}/pin", s.setPostPinned)
			r.Delete("/posts/{id}", s.adminDeletePost)
			r.Delete("/comments/{id}", s.adminDeleteComment)

			r.Group(func(r chi.Router) {
				r.Use(s.requireAdmin)
				r.Get("/users", s.listUsers)
				r.Post("/users/{id}/role", s.setUserRole)
				r.Post("/users/{id}/ban", s.setUserBanned)
			})
		})

		r.Route("/messages", func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/", s.listConversations)
			r.Get("/unread-count", s.unreadMessageCount)
			r.Get("/{username}", s.getConversationWithUser)
			r.With(s.limitByUser(limitMessage)).Post("/{username}", s.sendMessage)
		})

		r.Route("/notifications", func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/", s.listNotifications)
			r.Get("/unread-count", s.unreadNotificationCount)
			r.Post("/read-all", s.markAllNotificationsRead)
			r.Post("/{id}/read", s.markNotificationRead)
		})

		r.Get("/search", s.search)
		r.With(s.requireAuth).Post("/upload", s.upload)
		r.Get("/categories", s.listCategories)
		r.Get("/health", s.health)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}
