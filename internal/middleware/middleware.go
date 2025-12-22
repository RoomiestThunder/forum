package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"forum/internal/errors"
	"forum/internal/handlers"
)

// Handler is a wrapper for HTTP handlers to support middleware chaining.
type Handler func(http.ResponseWriter, *http.Request) error

// ServeHTTP implements the http.Handler interface.
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h(w, r); err != nil {
		handleError(w, err)
	}
}

// Middleware is a function that wraps a Handler.
type Middleware func(Handler) Handler

// Chain applies multiple middleware to a handler in reverse order.
func Chain(handler Handler, middlewares ...Middleware) Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// RecoveryMiddleware recovers from panics and returns a 500 error.
func RecoveryMiddleware(logger *slog.Logger) Middleware {
	return func(next Handler) Handler {
		return func(w http.ResponseWriter, r *http.Request) (err error) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						slog.Any("panic", rec),
						slog.String("method", r.Method),
						slog.String("path", r.RequestURI),
					)
					err = errors.ErrInternalServer().WithDetails("Internal server error occurred")
				}
			}()
			return next(w, r)
		}
	}
}

// LoggingMiddleware logs HTTP requests with timing information.
func LoggingMiddleware(logger *slog.Logger) Middleware {
	return func(next Handler) Handler {
		return func(w http.ResponseWriter, r *http.Request) error {
			start := time.Now()

			// Wrap response writer to capture status code
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			err := next(rw, r)

			duration := time.Since(start)
			logger.Info("request completed",
				slog.String("method", r.Method),
				slog.String("path", r.RequestURI),
				slog.Int("status", rw.statusCode),
				slog.Duration("duration", duration),
			)

			return err
		}
	}
}

// AuthMiddleware requires user to be authenticated.
func AuthMiddleware(next Handler) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		userID := getUserIDFromSession(w, r)
		if userID == 0 {
			return errors.ErrUnauthorized()
		}

		// Store userID in request context for downstream handlers
		ctx := context.WithValue(r.Context(), "userID", userID)
		*r = *r.WithContext(ctx)

		return next(w, r)
	}
}

// OptionalAuthMiddleware extracts user info if authenticated, continues if not.
func OptionalAuthMiddleware(next Handler) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		userID := getUserIDFromSession(w, r)
		if userID != 0 {
			ctx := context.WithValue(r.Context(), "userID", userID)
			*r = *r.WithContext(ctx)
		}
		return next(w, r)
	}
}

// RateLimitMiddleware limits requests per IP using token bucket algorithm.
func RateLimitMiddleware(limiter *RateLimiter) Middleware {
	return func(next Handler) Handler {
		return func(w http.ResponseWriter, r *http.Request) error {
			ip := r.RemoteAddr
			if !limiter.Allow(ip) {
				return errors.ErrTooManyRequests()
			}
			return next(w, r)
		}
	}
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// getUserIDFromSession extracts user ID from session cookie.
func getUserIDFromSession(w http.ResponseWriter, r *http.Request) int {
	user := handlers.GetCurrentUser(r)
	if user == nil {
		return 0
	}
	return user.ID
}

// handleError converts AppError to HTTP response.
func handleError(w http.ResponseWriter, err error) {
	if appErr, ok := err.(*errors.AppError); ok {
		w.WriteHeader(appErr.HTTPStatusCode())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		handlers.Render500(w, appErr.Message)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		handlers.Render500(w, "Internal Server Error")
	}
}

// RateLimiter implements token bucket rate limiting.
type RateLimiter struct {
	buckets map[string]*tokenBucket
	limit   int           // tokens per second
	burst   int           // maximum burst
	reset   time.Duration // reset interval
}

type tokenBucket struct {
	tokens    float64
	lastSeen  time.Time
	lastReset time.Time
}

// NewRateLimiter creates a new rate limiter with specified requests per second.
func NewRateLimiter(requestsPerSecond, burst int) *RateLimiter {
	return &RateLimiter{
		buckets: make(map[string]*tokenBucket),
		limit:   requestsPerSecond,
		burst:   burst,
		reset:   time.Minute, // clean up old buckets after 1 minute of inactivity
	}
}

// Allow checks if a request from the given key (IP) is allowed.
func (rl *RateLimiter) Allow(key string) bool {
	now := time.Now()
	bucket, exists := rl.buckets[key]

	if !exists || now.Sub(bucket.lastReset) > rl.reset {
		rl.buckets[key] = &tokenBucket{
			tokens:    float64(rl.burst),
			lastSeen:  now,
			lastReset: now,
		}
		bucket = rl.buckets[key]
	}

	// Add tokens based on time elapsed
	elapsed := now.Sub(bucket.lastSeen).Seconds()
	bucket.tokens = min(float64(rl.burst), bucket.tokens+elapsed*float64(rl.limit))
	bucket.lastSeen = now

	// Check if we have tokens available
	if bucket.tokens >= 1 {
		bucket.tokens--
		return true
	}

	return false
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// CORSMiddleware adds CORS headers (optional, for future API usage).
func CORSMiddleware() Middleware {
	return func(next Handler) Handler {
		return func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return nil
			}

			return next(w, r)
		}
	}
}

// RequestIDMiddleware adds a unique request ID to context.
func RequestIDMiddleware(next Handler) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		// Generate or extract request ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			// Generate a simple request ID (in production, use UUID)
			requestID = time.Now().Format("20060102150405") + "_" + r.RemoteAddr
		}

		ctx := context.WithValue(r.Context(), "requestID", requestID)
		*r = *r.WithContext(ctx)

		w.Header().Set("X-Request-ID", requestID)
		return next(w, r)
	}
}

// SecurityHeadersMiddleware adds important security headers.
func SecurityHeadersMiddleware() Middleware {
	return func(next Handler) Handler {
		return func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")

			return next(w, r)
		}
	}
}
