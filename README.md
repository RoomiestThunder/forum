# Forum

[![CI](https://github.com/RoomiestThunder/forum/actions/workflows/ci.yml/badge.svg)](https://github.com/RoomiestThunder/forum/actions/workflows/ci.yml)

A full-featured discussion forum: Go/chi JSON API on the backend, React SPA on the front. JWT auth, threaded comments, tags, bookmarks, drafts, @mentions, direct messages, live notifications over WebSocket, moderation tools with an audit log, email verification and password reset, GitHub sign-in, Redis caching and rate limiting, MinIO file storage, and Prometheus metrics.

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go 1.25, chi router |
| Frontend | React 19, TypeScript, Vite, React Router 7, TanStack Query 5, Zustand, Tailwind CSS 4 |
| Database | PostgreSQL 16 (prod) / SQLite (dev) |
| Auth | JWT (access 15 min + refresh 7 days, atomic single-use rotation) |
| Cache + rate limiting | Redis 7 (optional — falls back to an in-process limiter) |
| File storage | MinIO / S3 (optional — uploads disabled without it) |
| Email | SMTP via `net/smtp` (optional — logs instead of sending without it) |
| OAuth | GitHub (optional — hidden from the UI without it) |
| Real-time | WebSocket (gorilla/websocket), per-user event hub |
| Metrics | Prometheus + Grafana |
| CI/CD | GitHub Actions → GHCR |

Redis, MinIO, SMTP, and GitHub OAuth all degrade gracefully when unconfigured instead of failing to start — see [Configuration](#configuration).

## Features

- **Posts** with markdown, categories, tags, an optional image, likes/dislikes, bookmarks, pinning, and **drafts** (saved privately, published later)
- **Threaded comments** — replies nest up to five levels deep; a deleted comment that has replies stays as a `[deleted]` placeholder so the thread keeps its shape
- **@mentions** with autocomplete, and **notifications** (replies, comments, likes, mentions) delivered live over WebSocket
- **Direct messages** between users, and **blocking** — a blocked user can't message you or notify you
- **Moderation** — reports, moderator and admin roles, bans that take effect immediately (including live connections), and an **audit log** of every staff action
- **Accounts** — email verification, password reset, GitHub sign-in, avatars and bios
- Full-text search (PostgreSQL `tsvector`; `LIKE` on SQLite)

## Quick Start

### Full stack with Docker Compose

```bash
docker-compose up --build
```

| Service | URL |
|---|---|
| Forum | http://localhost:8080 |
| API | http://localhost:8080/api/v1 |
| Mailpit (dev SMTP inbox) | http://localhost:8025 |
| Grafana | http://localhost:3000 (admin/admin) |
| Prometheus | http://localhost:9090 |
| MinIO console | http://localhost:9001 (minioadmin/minioadmin) |

The first user to register becomes the admin.

### Local development

Backend and frontend run as two processes; Vite proxies `/api` (and the WebSocket) to the Go server.

```bash
# Terminal 1 — API (SQLite, no external services required)
go mod download
CGO_ENABLED=1 go build -o forum ./cmd/forum
./forum

# Terminal 2 — SPA with hot reload
cd web
npm ci
npm run dev
# Visit http://localhost:5173
```

The server starts fine without a frontend build — it logs a warning and answers non-API paths with a note on how to build it. The Go binary also serves a production build of the SPA directly (`go:embed`): run `npm run build` inside `web/`, rebuild the binary, and `./forum` alone serves the whole app at http://localhost:8080.

Without `JWT_SECRET` set, a random one is generated at startup, so sessions end whenever the server restarts. Set it (see below) to keep them.

## REST API

All endpoints return JSON. Authenticated endpoints require `Authorization: Bearer <access_token>`. Errors look like `{"error": "message"}`; rate-limited requests get `429` with a `Retry-After` header.

### Auth & account

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/auth/register` | — | Register, returns token pair |
| POST | `/api/v1/auth/login` | — | Login by email or username, returns token pair |
| POST | `/api/v1/auth/refresh` | — | Exchange a refresh token for a new pair (the old one is consumed) |
| POST | `/api/v1/auth/logout` | ✓ | Revoke refresh tokens |
| POST | `/api/v1/auth/verify-email` | — | Confirm email via token |
| POST | `/api/v1/auth/resend-verification` | ✓ | Resend verification email |
| POST | `/api/v1/auth/forgot-password` | — | Request a reset email (always 200, no enumeration) |
| POST | `/api/v1/auth/reset-password` | — | Reset password via token, revokes all sessions |
| GET | `/api/v1/auth/providers` | — | Which OAuth providers are enabled |
| GET | `/api/v1/auth/github` | — | Start GitHub OAuth flow |
| GET | `/api/v1/auth/github/callback` | — | GitHub OAuth callback |

Usernames are 3–20 letters, digits or underscores and unique ignoring case; passwords are 8–72 bytes.

### Profile

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/me` | ✓ | Current user |
| PUT | `/api/v1/me/bio` | ✓ | Update bio |
| POST | `/api/v1/me/avatar` | ✓ | Upload avatar (multipart `file`) |
| GET | `/api/v1/users/{username}` | — | Public profile |
| GET | `/api/v1/users/search?q=` | — | Username prefix search (mention autocomplete) |

### Posts & comments

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/posts` | optional | List posts (paginated, filterable — see below) |
| POST | `/api/v1/posts` | ✓ | Create post (`"draft": true` to save as a draft) |
| GET | `/api/v1/posts/{id}` | optional | Get post + comments (a flat list with `parent_id`/`depth`) |
| PUT | `/api/v1/posts/{id}` | ✓ | Update post (owner only) |
| DELETE | `/api/v1/posts/{id}` | ✓ | Delete post (owner only) |
| POST | `/api/v1/posts/{id}/publish` | ✓ | Publish a draft (owner only) |
| POST | `/api/v1/posts/{id}` | ✓ | Add a comment; `parent_id` makes it a reply |
| PUT | `/api/v1/comments/{id}` | ✓ | Edit comment (owner only) |
| DELETE | `/api/v1/comments/{id}` | ✓ | Delete comment (owner only); kept as a tombstone if it has replies |
| POST | `/api/v1/like/post` | ✓ | Toggle post like/dislike |
| POST | `/api/v1/like/comment` | ✓ | Toggle comment like/dislike |
| POST | `/api/v1/bookmarks` | ✓ | Toggle bookmark on a post |
| GET | `/api/v1/tags` | — | Autocomplete tag search |

#### Query parameters for `GET /api/v1/posts`

| Parameter | Values | Description |
|---|---|---|
| `page` | integer | Pagination (10 per page) |
| `filter` | `myposts`, `liked`, `saved`, `drafts` | Requires auth — own / liked / bookmarked / draft posts |
| `category` | category id | Filter by category |
| `tag` | tag name | Filter by tag |
| `author` | username | Filter by author |

Drafts never appear in any public listing, and a draft answers `404` to everyone but its author.

### Direct messages & blocking

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/messages` | ✓ | List conversations |
| GET | `/api/v1/messages/unread-count` | ✓ | Unread message count |
| GET | `/api/v1/messages/{username}` | ✓ | Messages with a user (marks them read) |
| POST | `/api/v1/messages/{username}` | ✓ | Send a message |
| GET | `/api/v1/blocks` | ✓ | List users you've blocked |
| PUT | `/api/v1/blocks/{username}` | ✓ | Block a user (idempotent) |
| DELETE | `/api/v1/blocks/{username}` | ✓ | Unblock a user |

A block stops messages in both directions (you must unblock to write to someone you blocked) and drops notifications caused by the blocked user. History stays readable. The blocked user gets a generic refusal that doesn't reveal the block.

### Notifications

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/notifications` | ✓ | List notifications |
| GET | `/api/v1/notifications/unread-count` | ✓ | Unread count |
| POST | `/api/v1/notifications/{id}/read` | ✓ | Mark one as read |
| POST | `/api/v1/notifications/read-all` | ✓ | Mark all as read |

Notification types: `new_comment`, `new_reply`, `new_like`, `new_mention`.

### Real-time (WebSocket)

Connect to `/api/v1/ws`, then send the access token as the **first frame**:

```json
{"type": "auth", "token": "<access token>"}
```

The server replies `{"type": "ready"}` and then pushes events (`{"type": "new_reply", "user_id": 1, "payload": {...}}`). A connection that doesn't authenticate within 5 seconds, or whose token is invalid or belongs to a banned account, is closed with status 1008. The token is deliberately not accepted in the URL, where it would end up in proxy and access logs. Events are hints to refetch, not a source of truth.

### Moderation & admin

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/reports` | ✓ | Report a post or comment |
| GET | `/api/v1/admin/reports` | staff | List reports |
| POST | `/api/v1/admin/reports/{id}/resolve` | staff | Resolve a report |
| POST | `/api/v1/admin/posts/{id}/pin` | staff | Pin/unpin a post |
| DELETE | `/api/v1/admin/posts/{id}` | staff | Delete any post |
| DELETE | `/api/v1/admin/comments/{id}` | staff | Delete any comment |
| GET | `/api/v1/admin/users` | admin | List users |
| POST | `/api/v1/admin/users/{id}/role` | admin | Grant/revoke moderator or admin |
| POST | `/api/v1/admin/users/{id}/ban` | admin | Ban/unban a user |
| GET | `/api/v1/admin/audit` | admin | Audit log of staff actions, newest first |

Every staff action (pin, delete, ban, role change, report resolution) is recorded with its actor and a snapshot of what was acted on — a deleted post's title, author and the start of its text — since the thing itself is gone afterwards.

### Other

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/search?q=...` | — | Full-text search (PostgreSQL tsvector / SQLite LIKE) |
| GET | `/api/v1/categories` | — | List categories |
| POST | `/api/v1/upload` | ✓ | Upload an image (multipart `file`; JPEG, PNG, GIF or WebP, max 5 MB) |
| GET | `/api/v1/health` | — | Health check |
| GET | `/metrics` | — | Prometheus metrics |

### Rate limits

Limits are per client IP, or per user for authenticated actions. They are shared across instances when Redis is configured; otherwise (or if Redis is unreachable) each instance enforces them itself.

| Scope | Limit |
|---|---|
| Every API request | 60 / minute / IP |
| Login, register, reset password | 10 / minute / IP |
| Forgot password, resend verification | 5 / 15 minutes / IP |
| Create post or comment | 20 / minute / user |
| Send direct message | 30 / minute / user |
| Upload file or avatar | 10 / minute / user |
| WebSocket upgrade | 30 / minute / IP |

## Configuration

Copy `.env.example` to `.env` and adjust as needed.

```bash
cp .env.example .env
```

Key variables:

```bash
# Server
PORT=8080
# Set true ONLY behind a reverse proxy that sets X-Forwarded-For / X-Real-IP.
# Rate limits key on the client IP; without this they see the proxy's address.
TRUST_PROXY=false
# Set to "production" to make a weak JWT_SECRET refuse to start.
APP_ENV=

# Database — set DATABASE_URL for PostgreSQL, otherwise SQLite is used
DATABASE_URL=postgres://forum:forum@localhost:5432/forum?sslmode=disable
DB_PATH=forum.db

# JWT — at least 32 random characters, e.g. `openssl rand -hex 32`.
# Unset: a random secret is generated at startup (sessions end on restart).
JWT_SECRET=
JWT_ACCESS_DURATION_MIN=15
JWT_REFRESH_DURATION_DAYS=7

# Redis (optional)
REDIS_URL=redis://localhost:6379

# MinIO / S3 (optional)
MINIO_ENDPOINT=localhost:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=forum
# Where browsers fetch uploaded files from, including the bucket. Leave empty to
# derive it from MINIO_ENDPOINT, which only works if that is reachable from the
# browser (not for an internal name such as minio:9000).
MINIO_PUBLIC_URL=

# Email (optional — leave SMTP_HOST unset to log instead of send)
SMTP_HOST=
SMTP_PORT=1025
SMTP_FROM=forum@localhost

# GitHub OAuth (optional — leave both unset to hide the button)
GITHUB_CLIENT_ID=
GITHUB_CLIENT_SECRET=

# Public URL — used in email links and the OAuth redirect URI
BASE_URL=http://localhost:8080
```

Redis, MinIO, SMTP, and GitHub OAuth are all optional. The app starts without any of them; the affected feature (caching, uploads, real email delivery, GitHub sign-in) is simply disabled or falls back to a no-op.

A `JWT_SECRET` that is the placeholder from `.env.example`, or shorter than 32 characters, logs a warning at startup and, with `APP_ENV=production`, stops the server from starting: anyone who knows the secret can forge a login for any user.

## Database

### PostgreSQL (production)

Schema is applied via the numbered files in `migrations/` (`001` – `014`, each with a `.down.sql`; mounted as Postgres init scripts in `docker-compose.yml`). Full-text search uses `tsvector GENERATED ALWAYS` with a GIN index. `internal/database/schema_postgres.sql` holds the same schema in one file for tests and first-run setup; a test diffs the two so they can't drift apart.

### SQLite (development)

Schema is created automatically on first run, and a database file from an older build is upgraded in place (missing columns are added, and duplicate usernames from before they were unique are renamed). Full-text search falls back to `LIKE`.

## Project Structure

```
forum/
├── cmd/forum/                  # main.go wires everything, spa.go serves the embedded SPA
├── internal/
│   ├── models/                 # Shared domain types
│   ├── config/                 # Config from environment variables, with validation
│   ├── database/               # Store interface + SQLite + PostgreSQL implementations
│   │   └── dbtest/             #   test helper: throwaway stores of either kind
│   ├── auth/                   # JWT token manager
│   ├── api/                    # REST API v1 handlers (chi router), rate limits, validation
│   ├── mail/                   # SMTP mailer with logging fallback
│   ├── websocket/              # WebSocket hub (per-user delivery)
│   ├── cache/                  # Redis cache-aside + atomic rate-limit counters
│   ├── storage/                # MinIO/S3 image storage
│   ├── metrics/                # Prometheus metrics
│   ├── middleware/             # Security headers, in-process rate limiter
│   └── errors/                 # Typed application errors
├── web/                        # React SPA (Vite + TypeScript)
│   ├── src/pages/              # Route-level views
│   ├── src/components/         # Shared UI components
│   ├── src/lib/                # API client, TanStack Query hooks, types, helpers
│   ├── src/stores/             # Zustand auth store
│   └── dist/                   # Production build, embedded into the Go binary
├── migrations/                 # SQL migration files (up + down)
├── monitoring/                 # Prometheus + Grafana config
├── Dockerfile                  # Node build stage → Go build stage → runtime
├── docker-compose.yml
└── .env.example
```

## Testing

```bash
# Go tests. SQLite tests run anywhere; PostgreSQL and Redis tests are skipped
# when no server is configured (see below).
go test -short ./...

# With the race detector
go test -short -race ./...

# Frontend lint + tests + typecheck + build
cd web && npm run lint && npm test && npm run build
```

The store tests (`internal/database`) are written once against the `Store` interface and run against both SQLite and PostgreSQL, because the two are separate implementations of the same ~90 methods. To include PostgreSQL and Redis:

```bash
docker run -d -p 5432:5432 -e POSTGRES_USER=forum -e POSTGRES_PASSWORD=forum -e POSTGRES_DB=forum_test postgres:16-alpine
docker run -d -p 6379:6379 redis:7-alpine

export TEST_DATABASE_URL='postgres://forum:forum@localhost:5432/forum_test?sslmode=disable'
export REDIS_URL=redis://localhost:6379

go test -short ./...

# The whole API suite against PostgreSQL instead of SQLite. An unreachable
# database fails this run rather than skipping it.
TEST_STORE=postgres go test -short ./internal/api/...
```

Each PostgreSQL test works in its own schema inside that database and drops it afterwards, so nothing else in the database is touched.

## CI/CD

GitHub Actions on every push to `main` / `develop` and on pull requests to `main`:

1. **frontend** — `oxlint`, `vitest`, `tsc`, `vite build` (dependencies installed with `npm ci`); the built `web/dist` is passed to the next job as an artifact
2. **test** — `golangci-lint`, `go test -race` against PostgreSQL and Redis service containers, the API suite a second time with `TEST_STORE=postgres`, then `go build` with the real embedded SPA
3. **docker** — multi-stage image build + push to GHCR (main branch only)

## Security

- JWT access tokens (HS256 only, expiry required); opaque `crypto/rand` refresh tokens, consumed atomically on use so a token can't be redeemed twice, with a lifetime from `JWT_REFRESH_DURATION_DAYS`
- Bcrypt password hashing; password, email and username validated on registration, and the same password rule on reset
- Login takes the same time whether or not the account exists
- Usernames are unique ignoring case, so an account can't be impersonated by registering a near-duplicate
- Bans take effect immediately — checked on every request, and a ban closes the user's live WebSocket
- Role-gated admin/moderator middleware, self-demote and self-ban blocked, and an audit log of staff actions
- WebSocket authenticates with a first-frame token, never a URL parameter
- Per-IP and per-user rate limits (Redis-backed with an in-process fallback), with stricter limits on credential, email and upload endpoints; forwarding headers are only trusted when `TRUST_PROXY` is set
- Password reset never reveals whether an email exists, and resets revoke all outstanding refresh tokens
- GitHub sign-in links to an existing account only through an email GitHub has **verified** (never the profile's public email); if that account's email was never verified on our side - registration doesn't require it - linking also revokes its password, sessions and outstanding tokens, so whoever created it on someone else's address can't keep access. A banned user can't sign in this way either, and a failed lookup at GitHub never creates an account
- OAuth state-cookie CSRF protection on the GitHub login flow
- Uploads are validated by their magic bytes (not their name or declared type), stored under a generated key, and capped at 5 MB as they stream
- Outgoing mail refuses header injection (CR/LF in any header value) and encodes non-ASCII subjects
- Store errors are logged server-side; the client only ever sees a safe message, never database or storage details
- SQL injection prevention via parameterized queries; destructive operations run in transactions
- Markdown rendered client-side with `react-markdown` (no `dangerouslySetInnerHTML`)

## License

MIT License — see [LICENSE](LICENSE)

## Author

RoomiestThunder
