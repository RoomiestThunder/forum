package api

import (
	"context"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	appmw "forum/internal/middleware"
)

// limitPolicy is a named rate limit: at most limit requests per window for
// each key (an IP address or a user, depending on the middleware used).
type limitPolicy struct {
	name   string
	limit  int
	window time.Duration
}

// The policies are deliberately tiered. The global limit protects the API as
// a whole; the tighter ones sit on endpoints where a single request is
// expensive or abusable (password guessing, mail sending, spam).
var (
	// limitGlobal applies to every API request, per IP.
	limitGlobal = limitPolicy{name: "global", limit: 60, window: time.Minute}
	// limitAuth guards credential endpoints (login, register) against
	// password guessing and account-creation floods, per IP.
	limitAuth = limitPolicy{name: "auth", limit: 10, window: time.Minute}
	// limitEmail guards endpoints that make the server send mail, so they
	// can't be used to mail-bomb an address, per IP.
	limitEmail = limitPolicy{name: "email", limit: 5, window: 15 * time.Minute}
	// limitContent throttles creating posts and comments, per user.
	limitContent = limitPolicy{name: "content", limit: 20, window: time.Minute}
	// limitMessage throttles direct messages, per user.
	limitMessage = limitPolicy{name: "message", limit: 30, window: time.Minute}
	// limitUpload throttles file uploads, per user: each one is up to 5 MB of
	// bandwidth and storage.
	limitUpload = limitPolicy{name: "upload", limit: 10, window: time.Minute}
	// limitSocket throttles websocket upgrades, per IP. The upgrade sits
	// outside the API router, so it is not covered by limitGlobal.
	limitSocket = limitPolicy{name: "socket", limit: 30, window: time.Minute}
)

var allLimitPolicies = []limitPolicy{
	limitGlobal, limitAuth, limitEmail, limitContent, limitMessage, limitUpload, limitSocket,
}

// newLocalLimiters builds the in-process limiter for every policy. They are
// always created, even when Redis is configured: they are the fallback that
// keeps limits enforced if Redis becomes unreachable.
func newLocalLimiters() map[string]*appmw.RateLimiter {
	limiters := make(map[string]*appmw.RateLimiter, len(allLimitPolicies))
	for _, p := range allLimitPolicies {
		limiters[p.name] = appmw.NewRateLimiter(p.limit, p.window)
	}
	return limiters
}

// limitByIP rate limits by client address.
func (s *Server) limitByIP(p limitPolicy) func(http.Handler) http.Handler {
	return s.limit(p, func(r *http.Request) string { return "ip:" + s.clientIP(r) })
}

// limitByUser rate limits by authenticated user and must be chained after
// requireAuth. A request without a user falls back to its IP rather than
// sharing one bucket among every anonymous caller.
func (s *Server) limitByUser(p limitPolicy) func(http.Handler) http.Handler {
	return s.limit(p, func(r *http.Request) string {
		if id := userIDFromCtx(r); id != 0 {
			return "user:" + strconv.Itoa(id)
		}
		return "ip:" + s.clientIP(r)
	})
}

func (s *Server) limit(p limitPolicy, keyOf func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, retryAfter := s.allow(r.Context(), p, keyOf(r))
			if !allowed {
				secs := max(1, int(math.Ceil(retryAfter.Seconds())))
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				jsonError(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// allow consults Redis when it is configured and otherwise, or when Redis
// errors, the in-process limiter. Falling back instead of failing open means
// a Redis outage doesn't silently turn every limit off.
func (s *Server) allow(ctx context.Context, p limitPolicy, key string) (bool, time.Duration) {
	scoped := p.name + ":" + key
	if s.cache != nil {
		allowed, retryAfter, err := s.cache.Allow(ctx, "rate:"+scoped, p.limit, p.window)
		if err == nil {
			return allowed, retryAfter
		}
	}
	return s.limiters[p.name].Allow(scoped)
}

// clientIP returns the address to rate limit a request by. It is the TCP peer
// with the port stripped: RemoteAddr is "ip:port" with a fresh ephemeral port
// per connection, so using it raw would give every new connection its own
// bucket and make the limits trivially bypassable.
//
// Forwarding headers are honoured only when the server is explicitly told it
// sits behind a trusted reverse proxy; otherwise any client could pick its own
// rate-limit identity by sending them.
func (s *Server) clientIP(r *http.Request) string {
	if s.trustProxy {
		// The proxy appends the address it saw, so the last entry is the one
		// it vouches for; earlier entries are client-supplied.
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
				return ip
			}
		}
		if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
