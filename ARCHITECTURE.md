# Architecture

## Overview

Forum is a JSON API written in Go, paired with a React single-page app. The Go binary is self-contained: it serves `/api/v1/*` directly and serves the SPA's production build (embedded via `go:embed`) for everything else, with an `index.html` fallback for client-side routing. There's no server-rendered HTML and no cookie session state — the SPA talks to the API exclusively over JWT bearer tokens (and a WebSocket for live events).

```
┌──────────────────────────────────────────────┐
│                  React SPA                    │
│  React Router · TanStack Query · Zustand      │
│  (served as static assets, or via Vite in dev)│
└───────────────────┬────────────────────────────┘
                     │ fetch() with Authorization: Bearer <token>
                     │ WebSocket /api/v1/ws (token in the first frame)
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
   SQLite      (cache +     (images)      (mailer)
              rate limit)
```

Redis, MinIO, SMTP, and GitHub OAuth are all optional dependencies. Each follows the same degrade-gracefully pattern: if unconfigured or unreachable at startup, the app logs a warning and disables just that feature (cache/shared rate limits, uploads, real email delivery, GitHub sign-in) rather than failing to boot. The embedded frontend works the same way: with no build embedded the server still starts and serves the API.

## Packages

- **cmd/forum** — process bootstrap: loads config, wires the store/cache/storage/mailer/websocket hub into `api.Server`, mounts the SPA handler, starts the HTTP server with graceful shutdown (hub closed after the server).
- **internal/config** — environment-variable configuration with defaults and validation, including the checks on `JWT_SECRET` (a generated secret when unset, warnings for a weak one, refusal to start in production).
- **internal/api** — all `/api/v1/*` handlers, grouped by feature file (`posts.go`, `comments.go`, `messages.go`, `blocks.go`, `admin.go`, `audit.go`, `upload.go`, `account.go`, `oauth.go`, ...) plus the chi route tree in `router.go`, auth middleware in `middleware.go`, rate-limit policies in `ratelimit.go`, input rules in `validate.go` and the error helpers in `errors.go`.
- **internal/database** — the `Store` interface plus `SQLiteStore` and `PostgresStore` implementations. Every query goes through this interface; handlers never touch `database/sql` directly. `dbtest` builds throwaway stores for tests.
- **internal/auth** — JWT access-token issuance/verification and opaque refresh-token generation.
- **internal/mail** — SMTP sending with a logging fallback when unconfigured; builds RFC 5322 messages and refuses header injection.
- **internal/websocket** — the hub: tracks per-user connections, authenticates them, and delivers events.
- **internal/cache** — Redis-backed cache-aside for post listings and the atomic counter behind the shared rate limits.
- **internal/storage** — MinIO/S3 image storage: content sniffing, size cap, key generation, public URL.
- **internal/metrics** — Prometheus counters/histograms and the `/metrics` handler.
- **internal/middleware** — security headers and the in-process token-bucket rate limiter.
- **internal/errors** — typed `AppError` with HTTP status mapping.
- **web/** — the SPA. `lib/api.ts` is the fetch wrapper (attaches the bearer token, refreshes it once on a 401 and shares that refresh among concurrent requests); `lib/queries.ts` holds all TanStack Query hooks; `stores/auth.ts` is the Zustand store persisting tokens to `localStorage`.

## Design Patterns

### Repository pattern

`database.Store` is a single interface covering every persistence operation (users, posts, comments, tags, bookmarks, notifications, messages, blocks, reports, audit log, tokens, ...). Handlers depend on the interface, not a concrete driver, which is what makes it possible to run the full API test suite against an in-memory SQLite store with no external services — and against PostgreSQL with the same tests.

The two implementations are written by hand in different SQL dialects, so they can diverge in ways no single-database test would notice (SQLite's `INSERT OR IGNORE` swallows `CHECK` violations that PostgreSQL rejects, for one). The store tests are therefore written once and run against both.

### Graceful degradation for optional infrastructure

Redis, MinIO, SMTP, and GitHub OAuth are each wired the same way: `New(...)` returns `(nil, err)` or an unconfigured zero-value when the dependency isn't reachable/set, `main.go` logs a warning instead of `log.Fatal`, and the API layer checks for `nil` before using it (e.g. the rate limiter always has an in-process fallback, and `githubLogin`/`githubCallback` 404 when `s.githubOAuth == nil`). The API depends on a small `fileStore` interface rather than the MinIO client, assigned only when storage exists so "not configured" checks stay accurate.

### Typed application errors, and one place that answers them

```go
type AppError struct {
    Code    int
    Message string
    Err     error
    Details string
}
```

The store returns `*AppError` values (`NotFoundError`, `PermissionError`, `ValidationError`, `DatabaseError`, an account-taken 409). Handlers hand any store error to `Server.storeError`, the single place that turns one into a response: errors below 500 keep their status and a client-facing message; everything else is logged with the action, method, path and user, and the client gets a generic `failed to <action>`. Error text is never sent to the client — for a database error it names tables and driver internals.

### Middleware chain

Standard `net/http` middleware composed through chi: `Recoverer` → `SecurityHeaders` at the router root (`RealIP` is deliberately not used — it trusts forwarding headers from anyone), then per-route `requireAuth` / `optionalAuth` / `requireStaff` / `requireAdmin` for authorization and rate-limit middleware per policy.

### Rate limiting

Limits are named policies (`limitGlobal`, `limitAuth`, `limitEmail`, `limitContent`, `limitMessage`, `limitUpload`, `limitSocket`), each a count per window, keyed by client IP or by user. A policy is enforced in Redis when available — one Lua script does the increment and arms the expiry atomically, so a crash can't leave a key without a TTL that would lock it out forever — and otherwise, or when Redis errors, by an in-process token bucket per policy (idle buckets are evicted so memory is bounded). A Redis outage therefore never turns limits off.

The client IP is the TCP peer with the port stripped (the raw `RemoteAddr` changes with every connection, which would give each connection its own bucket). `X-Forwarded-For`/`X-Real-IP` are honoured only when `TRUST_PROXY` is set, since otherwise a client could choose its own identity.

## Auth Model

- **Registration/login** return a JWT access token (HS256, short-lived, 15 min default, expiry required) and an opaque refresh token (32 bytes of `crypto/rand`, base64-encoded — not a JWT). Registration validates the email (a bare address), username (3–20 letters, digits, underscores; unique ignoring case — enforced by a unique index on `LOWER(username)`) and password (8–72 bytes, bcrypt's limit). Login for an unknown account does the same bcrypt work as a wrong password so timing doesn't reveal which accounts exist.
- **Refresh** consumes the token: `DELETE ... RETURNING` deletes it and returns it in one statement, so of any number of simultaneous attempts exactly one succeeds and a replayed token gets a 401. A new pair is issued; its lifetime comes from `JWT_REFRESH_DURATION_DAYS`.
- **`requireAuth`** re-fetches the user from the database on every request (not just at token issuance) so a ban takes effect immediately instead of waiting for the access token to expire. A ban also disconnects the user's live WebSocket.
- **First user to register becomes admin.** There's no seed script or CLI for bootstrapping roles.
- **Password reset / email verification** use single-use tokens stored in dedicated tables, sent via `internal/mail`. `forgotPassword` always returns 200 regardless of whether the email exists, to avoid account enumeration. A successful reset revokes all of the user's refresh tokens, and a new password is validated before the token is spent.
- **GitHub OAuth** is a standard authorization-code flow (`golang.org/x/oauth2`) with a state cookie for CSRF protection. On callback, the server finds-or-creates a user by GitHub account and redirects to the SPA with the token pair in the URL fragment (not JSON — a `fetch()` call can't follow a 302 to an external OAuth provider and get the tokens back). Only a primary-or-any **verified** address from `/user/emails` is used to attach the identity to an existing account; the public profile email is ignored, since anyone can type any address into it. Because registration doesn't verify email, an existing account may have been created by someone who doesn't own the address, with a password they know; linking therefore strips an unverified account's password, refresh tokens and outstanding reset/verification tokens (in one transaction) and marks the email verified, so the person who proved ownership at GitHub is the only one left with access. Non-200 answers from GitHub are errors, a banned account gets no tokens, GitHub logins are reshaped to the username rules (hyphens become underscores, 20 characters at most), and username collisions are resolved by retrying the insert against the unique index rather than by a check-then-insert that a concurrent sign-up could beat.
- **`JWT_SECRET`** if unset is generated at startup; the shipped placeholder and anything under 32 characters is warned about and, with `APP_ENV=production`, refuses to start.

## Data Model

```
users (1) ──┬── (N) posts
            ├── (N) comments
            ├── (N) post_likes / comment_likes
            ├── (N) bookmarks
            ├── (N) notifications
            ├── (N) reports (as reporter)
            ├── (N) user_blocks (as blocker, and as blocked)
            ├── (N) audit_log (as actor)
            ├── (N) conversations ── (N) messages
            ├── (N) oauth_identities
            └── (N) refresh_tokens / email_verification_tokens / password_reset_tokens

posts (1) ──┬── (N) comments ── (N) comments (replies, via parent_id)
            ├── (N) post_categories ── (1) categories
            ├── (N) post_tags ── (1) tags
            └── (N) bookmarks

comments (1) ── (N) comment_likes
```

| Table | Purpose |
|---|---|
| users | accounts — credentials, bio, avatar, role flags, ban state; `username` unique ignoring case |
| posts | title, markdown content, optional image, pinned and draft flags |
| comments | markdown content on a post; `parent_id` and `depth` for replies, `deleted` for tombstones |
| categories, post_categories | fixed post categories |
| tags, post_tags | free-form tags with autocomplete search |
| bookmarks | per-user saved posts |
| post_likes, comment_likes | up/down votes |
| notifications | persisted events (comment, reply, like, mention) surfaced via the bell + WebSocket push |
| conversations, messages | direct messages; a conversation is keyed by the (lower id, higher id) pair |
| user_blocks | who has blocked whom |
| reports | user-filed reports on a post or comment, with resolution state |
| audit_log | staff actions: actor, action, target and a JSON snapshot of what was acted on |
| refresh_tokens | opaque refresh tokens, each with its own expiry |
| email_verification_tokens, password_reset_tokens | single-use account-security tokens |
| oauth_identities | provider + provider user id linked to a local account |
| uploads | a record per uploaded image |

### Threaded comments

Comments are stored and served as a flat, oldest-first list; each carries `parent_id` and a stored `depth`, and the client rebuilds the tree. Nesting is capped at five levels below a top-level comment. Deleting a comment that has replies replaces it with a tombstone (`deleted = TRUE`, content cleared, author hidden when served) so the replies keep their place; when the last reply under a tombstone is deleted, the tombstone and any chain of childless tombstones above it are removed. The whole delete is one transaction, with the ownership check first.

### Schema management

PostgreSQL's schema exists in two forms that must describe the same database: the numbered files in `migrations/` (applied by docker-compose and real deployments) and `internal/database/schema_postgres.sql` (applied by tests and first-run setup). They're edited by hand, so tests apply both to empty schemas and diff every column, constraint and index that PostgreSQL reports, check that rolling everything back leaves nothing, and check that each migration is mounted in `docker-compose.yml`.

SQLite has no migration files: the schema is created on first run, and a list of late-added columns is applied idempotently on open so a database file from an older build is upgraded in place. Migration 013 (unique usernames) renames existing duplicates first — the oldest account keeps the name — so the index can be built.

## Real-time Events

`internal/websocket` holds a hub: a mutex-guarded map from user id to that user's connections. Nothing in it needs a dispatcher goroutine, because delivery is a non-blocking send into each connection's buffered channel — a handler calling `Notify` can never be stalled by the hub. A connection whose buffer fills is dropped rather than allowed to hold up the rest.

A client connects to `/api/v1/ws` and authenticates with its **first frame** (`{"type":"auth","token":"..."}`) within five seconds; the same checks as `requireAuth` apply, including the ban check. The token is not accepted in the URL, which would put it in proxy and access logs. The hub then sends `{"type":"ready"}` and begins delivering events. Removal is idempotent (a connection can be removed by its read loop, a slow-consumer drop and a ban at once, and the active-connections gauge and the channel close each happen once), `Disconnect(userID)` closes a banned user's sockets, and `Close()` on shutdown ends everything and refuses new connections.

When an action creates a notification (someone replies to your comment, mentions you, likes your post, messages you), the handler persists it via `Store` and pushes it to the hub. Events carry just enough to identify what changed; the SPA reacts by refetching, so the socket is never the source of truth. Notifications caused by a user the recipient has blocked are dropped before either step.

## Testing

- **`internal/api`** — handler-level tests through real HTTP requests, against an in-memory SQLite store by default. The same suite runs against PostgreSQL with `TEST_STORE=postgres` (CI does this), covering auth, CRUD, threads, filters, moderation and the audit log, rate limits, uploads (with a fake object store), and account-security flows end to end.
- **`internal/database`** — tests written once against the `Store` interface and run for both implementations (PostgreSQL when `TEST_DATABASE_URL` is set), plus the migration/schema checks and the SQLite upgrade-in-place tests. `dbtest` provides each test its own empty store; a PostgreSQL test gets its own schema in the shared database and drops it afterwards.
- **Concurrency** — properties that only fail under contention (a refresh token redeemable once, the hub, the rate limiter) are tested with many goroutines released from a common gate, over several rounds, and were checked to fail when the property is broken.
- **`internal/websocket`, `internal/cache`, `internal/mail`, `internal/storage`, `internal/auth`, `internal/middleware`, `internal/config`, `internal/errors`, `cmd/forum`** — focused unit tests; the cache tests use a real Redis (skipped when none is reachable) because the rate limiter is a Lua script, and the mail tests run the real SMTP client against an in-process fake server.
- **web/** — `oxlint` + `tsc -b` catch type and lint errors; Vitest + React Testing Library cover the API client (including token refresh), hooks, the comment tree, key components and pages, mocking `lib/api` at the function level rather than stubbing `fetch` directly.

Run everything with `go test -short ./...` (SQLite only, fast); see the README for adding PostgreSQL and Redis.

## Deployment

`Dockerfile` is a three-stage build: a Node stage installs dependencies (`npm ci`) and runs `npm run build` to produce `web/dist`, a Go stage copies that into the build context so `go:embed` picks it up and compiles a static binary, and a minimal Alpine runtime stage just copies the binary. `docker-compose.yml` wires up Postgres, Redis, MinIO, Mailpit (dev SMTP inbox), Prometheus, and Grafana alongside the app; it sets `MINIO_PUBLIC_URL` because the app reaches MinIO by an internal name browsers can't resolve.

CI (`.github/workflows/ci.yml`) mirrors that shape: a `frontend` job lints, tests and builds the SPA and uploads `web/dist` as an artifact; a `test` job downloads it, runs `golangci-lint` and `go test -race` against Postgres+Redis service containers, runs the API suite again against PostgreSQL, then builds the Go binary with the real embedded frontend; a `docker` job (main branch only) builds and pushes the same multi-stage image to GHCR.
