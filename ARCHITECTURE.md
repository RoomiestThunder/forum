# Architecture

## Overview

Forum is a JSON API written in Go, paired with a React single-page app. The Go binary is self-contained: it serves `/api/v1/*` directly and serves the SPA's production build (embedded via `go:embed`) for everything else, with an `index.html` fallback for client-side routing. There's no server-rendered HTML and no cookie session state — the SPA talks to the API exclusively over JWT bearer tokens.

```
┌──────────────────────────────────────────────┐
│                  React SPA                    │
│  React Router · TanStack Query · Zustand      │
│  (served as static assets, or via Vite in dev)│
└───────────────────┬────────────────────────────┘
                     │ fetch() with Authorization: Bearer <token>
                     ▼
┌──────────────────────────────────────────────┐
│              chi router (/api/v1)              │
│   rate limit → auth middleware → handler        │
├──────────────────────────────────────────────┤
│         Store interface (database ops)          │
│         SQLiteStore  │  PostgresStore            │
└──────────────────────────────────────────────┘
        │            │            │           │
        ▼            ▼            ▼           ▼
   Postgres/     Redis        MinIO        SMTP
   SQLite      (cache +     (file        (mailer)
              rate limit)   storage)
```

Redis, MinIO, SMTP, and GitHub OAuth are all optional dependencies. Each follows the same degrade-gracefully pattern: if unconfigured or unreachable at startup, the app logs a warning and disables just that feature (cache/rate-limit, uploads, real email delivery, GitHub sign-in) rather than failing to boot.

## Packages

- **cmd/forum** — process bootstrap: loads config, wires the store/cache/storage/mailer/websocket hub into `api.Server`, mounts the SPA handler, starts the HTTP server with graceful shutdown.
- **internal/config** — environment-variable configuration with defaults and validation.
- **internal/api** — all `/api/v1/*` handlers, grouped by feature file (`posts.go`, `comments.go`, `profile.go`, `tags.go`, `notifications.go`, `admin.go`, `account.go`, `oauth.go`, ...) plus the chi route tree in `router.go` and auth middleware in `middleware.go`.
- **internal/database** — the `Store` interface plus `SQLiteStore` and `PostgresStore` implementations. Every query goes through this interface; handlers never touch `database/sql` directly.
- **internal/auth** — JWT access/refresh token issuance and verification.
- **internal/mail** — SMTP sending with a logging fallback when unconfigured.
- **internal/websocket** — a hub that tracks per-user connections and pushes notification events.
- **internal/cache** — Redis-backed cache-aside for post listings and a token-bucket rate limiter.
- **internal/storage** — MinIO/S3 client for avatar and post image uploads.
- **internal/metrics** — Prometheus counters/histograms and the `/metrics` handler.
- **internal/middleware** — security headers and the in-memory rate limiter fallback.
- **internal/errors** — typed `AppError` with HTTP status mapping, so handlers return errors instead of writing responses directly.
- **web/** — the SPA. `lib/api.ts` is the fetch wrapper (attaches the bearer token, retries once after a silent refresh on 401); `lib/queries.ts` holds all TanStack Query hooks; `stores/auth.ts` is the Zustand store persisting tokens to `localStorage`.

## Design Patterns

### Repository pattern

`database.Store` is a single interface covering every persistence operation (users, posts, comments, tags, bookmarks, notifications, reports, tokens, ...). Handlers depend on the interface, not a concrete driver, which is what makes it possible to run the full API test suite against an in-memory SQLite store with no external services.

### Graceful degradation for optional infrastructure

Redis, MinIO, SMTP, and GitHub OAuth are each wired the same way: `New(...)` returns `(nil, err)` or an unconfigured zero-value when the dependency isn't reachable/set, `main.go` logs a warning instead of `log.Fatal`, and the API layer checks for `nil` before using it (e.g. `rateLimit` falls back to an in-memory limiter when `s.cache == nil`; `githubLogin`/`githubCallback` 404 when `s.githubOAuth == nil`).

### Typed application errors

```go
type AppError struct {
    Code    int
    Message string
    Err     error
    Details string
}
```

Handlers return `*AppError` (via helpers like `apperrors.NotFoundError`, `apperrors.PermissionError`) instead of writing the HTTP response inline, keeping status-code decisions in one place.

### Middleware chain

Standard `net/http` middleware composed through chi: `RealIP` → `Recoverer` → `SecurityHeaders` at the router root, then per-route `requireAuth` / `optionalAuth` / `requireStaff` / `requireAdmin` for authorization, and a rate-limit middleware that prefers Redis but falls back to an in-memory bucket.

## Auth Model

- **Registration/login** return a JWT access token (short-lived, 15 min default) and an opaque refresh token (32 bytes of `crypto/rand`, base64-encoded — not a JWT, so two tokens issued in the same instant can't collide).
- **Refresh** rotates the token: the old refresh token is invalidated and a new pair is issued on every use.
- **`requireAuth`** re-fetches the user from the database on every request (not just at token issuance) so a ban takes effect immediately instead of waiting for the access token to expire.
- **First user to register becomes admin.** There's no seed script or CLI for bootstrapping roles.
- **Password reset / email verification** use single-use tokens stored in dedicated tables, sent via `internal/mail`. `forgotPassword` always returns 200 regardless of whether the email exists, to avoid account enumeration. A successful reset revokes all of the user's refresh tokens.
- **GitHub OAuth** is a standard authorization-code flow (`golang.org/x/oauth2`) with a state cookie for CSRF protection. On callback, the server finds-or-creates a user by GitHub account and redirects to the SPA with the token pair in the URL fragment (not JSON — a `fetch()` call can't follow a 302 to an external OAuth provider and get the tokens back).

## Data Model

```
users (1) ──┬── (N) posts
            ├── (N) comments
            ├── (N) post_likes / comment_likes
            ├── (N) bookmarks
            ├── (N) notifications
            ├── (N) reports (as reporter)
            ├── (N) oauth_identities
            └── (N) refresh_tokens / email_verification_tokens / password_reset_tokens

posts (1) ──┬── (N) comments
            ├── (N) post_categories ── (1) categories
            ├── (N) post_tags ── (1) tags
            └── (N) bookmarks

comments (1) ── (N) comment_likes
```

| Table | Purpose |
|---|---|
| users | accounts — credentials, bio, avatar, role flags, ban state |
| posts | title, markdown content, optional image, pinned flag |
| comments | markdown content on a post |
| categories, post_categories | fixed post categories |
| tags, post_tags | free-form tags with autocomplete search |
| bookmarks | per-user saved posts |
| post_likes, comment_likes | up/down votes |
| notifications | persisted events (reply, like, mention, ...) surfaced via the bell + WebSocket push |
| reports | user-filed reports on a post or comment, with resolution state |
| refresh_tokens | opaque JWT refresh tokens |
| email_verification_tokens, password_reset_tokens | single-use account-security tokens |
| oauth_identities | provider + provider user id linked to a local account |

Both `internal/database/schema_postgres.sql` (embedded, used by integration tests) and the numbered files in `migrations/` (mounted as Postgres init scripts in Docker Compose) describe this schema — they're kept in sync by hand whenever a migration is added.

## Real-time Notifications

`internal/websocket` runs a hub goroutine that tracks live connections per user ID. When an action creates a notification (someone replies to your post, likes your comment, etc.), the handler persists it via `Store` and pushes it to the hub, which forwards it to any open WebSocket connection for that user. The SPA connects via `GET /api/v1/ws?token=...` and updates the notification bell without polling.

## Testing

- **`internal/api`** — handler-level tests against an in-memory SQLite store (`:memory:`, `SetMaxOpenConns(1)` so every test connection shares the same database), covering auth, CRUD, filters, moderation, and account-security flows end to end through real HTTP requests.
- **`internal/config`**, **`internal/errors`**, **`internal/middleware`** — focused unit tests.
- **`internal/tests`** — integration tests that spin up real PostgreSQL via testcontainers, for the code paths that differ between the SQLite and Postgres store implementations (full-text search, in particular).
- **web/** — `oxlint` + `tsc -b` catch type and lint errors; Vitest + React Testing Library cover key components (auth forms, post cards, comment mentions/editing), mocking `lib/api` at the function level rather than stubbing `fetch` directly.

Run everything with `go test -short ./...` (SQLite tests only, fast) or add `./internal/tests/...` for the Postgres integration tests (requires Docker).

## Deployment

`Dockerfile` is a three-stage build: a Node stage installs dependencies and runs `npm run build` to produce `web/dist`, a Go stage copies that into the build context so `go:embed` picks it up and compiles a static binary, and a minimal Alpine runtime stage just copies the binary. `docker-compose.yml` wires up Postgres, Redis, MinIO, Mailpit (dev SMTP inbox), Prometheus, and Grafana alongside the app.

CI (`.github/workflows/ci.yml`) mirrors that shape: a `frontend` job builds and lints the SPA and uploads `web/dist` as an artifact; a `test` job downloads it, runs `golangci-lint` and `go test -race` against Postgres+Redis service containers, then builds the Go binary with the real embedded frontend; a `docker` job (main branch only) builds and pushes the same multi-stage image to GHCR.
