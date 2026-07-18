# Forum

[![CI](https://github.com/RoomiestThunder/forum/actions/workflows/ci.yml/badge.svg)](https://github.com/RoomiestThunder/forum/actions/workflows/ci.yml)

A full-featured discussion forum: Go/chi JSON API on the backend, React SPA on the front. JWT auth, tags, bookmarks, live notifications over WebSocket, moderation tools, email verification and password reset, GitHub sign-in, Redis caching, MinIO file storage, and Prometheus metrics.

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go 1.25, chi router |
| Frontend | React 19, TypeScript, Vite, React Router 7, TanStack Query 5, Zustand, Tailwind CSS 4 |
| Database | PostgreSQL 16 (prod) / SQLite (dev) |
| Auth | JWT (access 15 min + refresh 7 days, rotation on use) |
| Cache + rate limiting | Redis 7 (optional — falls back to an in-memory limiter) |
| File storage | MinIO / S3 (optional — uploads disabled without it) |
| Email | SMTP via `net/smtp` (optional — logs instead of sending without it) |
| OAuth | GitHub (optional — hidden from the UI without it) |
| Real-time | WebSocket (gorilla/websocket), per-user notification hub |
| Metrics | Prometheus + Grafana |
| CI/CD | GitHub Actions → GHCR |

Redis, MinIO, SMTP, and GitHub OAuth all degrade gracefully when unconfigured instead of failing to start — see [Configuration](#configuration).

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

### Local development

Backend and frontend run as two processes; Vite proxies `/api` to the Go server.

```bash
# Terminal 1 — API (SQLite, no external services required)
go mod download
CGO_ENABLED=1 go build -o forum ./cmd/forum
./forum

# Terminal 2 — SPA with hot reload
cd web
npm install
npm run dev
# Visit http://localhost:5173
```

The Go binary also serves a production build of the SPA directly (`go:embed`) — running `npm run build` inside `web/` and then starting `./forum` alone is enough to browse the full app at http://localhost:8080 with no separate frontend server.

## REST API

All endpoints return JSON. Authenticated endpoints require `Authorization: Bearer <access_token>`.

### Auth & account

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/auth/register` | — | Register, returns token pair |
| POST | `/api/v1/auth/login` | — | Login, returns token pair |
| POST | `/api/v1/auth/refresh` | — | Rotate refresh token |
| POST | `/api/v1/auth/logout` | ✓ | Revoke refresh tokens |
| POST | `/api/v1/auth/verify-email` | — | Confirm email via token |
| POST | `/api/v1/auth/resend-verification` | ✓ | Resend verification email |
| POST | `/api/v1/auth/forgot-password` | — | Request a reset email (always 200, no enumeration) |
| POST | `/api/v1/auth/reset-password` | — | Reset password via token, revokes all sessions |
| GET | `/api/v1/auth/providers` | — | Which OAuth providers are enabled |
| GET | `/api/v1/auth/github` | — | Start GitHub OAuth flow |
| GET | `/api/v1/auth/github/callback` | — | GitHub OAuth callback |

### Profile

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/me` | ✓ | Current user |
| PUT | `/api/v1/me/bio` | ✓ | Update bio |
| POST | `/api/v1/me/avatar` | ✓ | Upload avatar (multipart) |
| GET | `/api/v1/users/{username}` | — | Public profile |

### Posts & comments

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/posts` | optional | List posts (paginated, filterable — see below) |
| POST | `/api/v1/posts` | ✓ | Create post |
| GET | `/api/v1/posts/{id}` | optional | Get post + comments |
| PUT | `/api/v1/posts/{id}` | ✓ | Update post (owner only) |
| DELETE | `/api/v1/posts/{id}` | ✓ | Delete post (owner only) |
| POST | `/api/v1/posts/{id}` | ✓ | Add comment |
| PUT | `/api/v1/comments/{id}` | ✓ | Edit comment (owner only) |
| DELETE | `/api/v1/comments/{id}` | ✓ | Delete comment (owner only) |
| POST | `/api/v1/like/post` | ✓ | Toggle post like/dislike |
| POST | `/api/v1/like/comment` | ✓ | Toggle comment like/dislike |
| POST | `/api/v1/bookmarks` | ✓ | Toggle bookmark on a post |
| GET | `/api/v1/tags` | — | Autocomplete tag search |

#### Query parameters for `GET /api/v1/posts`

| Parameter | Values | Description |
|---|---|---|
| `page` | integer | Pagination (10 per page) |
| `filter` | `myposts`, `liked`, `saved` | Requires auth — own / liked / bookmarked posts |
| `category` | category id | Filter by category |
| `tag` | tag name | Filter by tag |
| `author` | username | Filter by author |

### Notifications

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/notifications` | ✓ | List notifications |
| GET | `/api/v1/notifications/unread-count` | ✓ | Unread count |
| POST | `/api/v1/notifications/{id}/read` | ✓ | Mark one as read |
| POST | `/api/v1/notifications/read-all` | ✓ | Mark all as read |
| GET | `/api/v1/ws?token=...` | ✓ | WebSocket stream (live notification push) |

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
| POST | `/api/v1/admin/users/{id}/role` | admin | Grant/revoke moderator |
| POST | `/api/v1/admin/users/{id}/ban` | admin | Ban/unban a user |

The first user to register becomes an admin automatically — there's no seed script.

### Other

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/search?q=...` | — | Full-text search (PostgreSQL tsvector / SQLite LIKE) |
| GET | `/api/v1/categories` | — | List categories |
| POST | `/api/v1/upload` | ✓ | Upload file (multipart, max 5 MB) |
| GET | `/api/v1/health` | — | Health check |
| GET | `/metrics` | — | Prometheus metrics |

## Configuration

Copy `.env.example` to `.env` and adjust as needed.

```bash
cp .env.example .env
```

Key variables:

```bash
# Server
PORT=8080

# Database — set DATABASE_URL for PostgreSQL, otherwise SQLite is used
DATABASE_URL=postgres://forum:forum@localhost:5432/forum?sslmode=disable
DB_PATH=forum.db

# JWT
JWT_SECRET=change-me-in-production
JWT_ACCESS_DURATION_MIN=15
JWT_REFRESH_DURATION_DAYS=7

# Redis (optional)
REDIS_URL=redis://localhost:6379

# MinIO / S3 (optional)
MINIO_ENDPOINT=localhost:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=forum

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

## Database

### PostgreSQL (production)

Schema is applied via the numbered files in `migrations/` (mounted as Postgres init scripts in `docker-compose.yml`). Full-text search uses `tsvector GENERATED ALWAYS` with a GIN index.

### SQLite (development)

Schema is created automatically on first run. Full-text search falls back to `LIKE`.

## Project Structure

```
forum/
├── cmd/forum/                  # main.go wires everything, spa.go serves the embedded SPA
├── internal/
│   ├── models/                 # Shared domain types
│   ├── config/                 # Config from environment variables
│   ├── database/               # Store interface + SQLite + PostgreSQL implementations
│   ├── auth/                   # JWT token manager
│   ├── api/                    # REST API v1 handlers (chi router)
│   ├── mail/                   # SMTP mailer with logging fallback
│   ├── websocket/               # WebSocket hub (per-user broadcast)
│   ├── cache/                  # Redis cache-aside + rate limiting
│   ├── storage/                # MinIO/S3 file upload
│   ├── metrics/                # Prometheus metrics
│   ├── middleware/              # Security headers, rate limiting
│   ├── errors/                 # Typed application errors
│   └── tests/                  # Integration tests
├── web/                        # React SPA (Vite + TypeScript)
│   ├── src/pages/               # Route-level views
│   ├── src/components/          # Shared UI components
│   ├── src/lib/                 # API client, TanStack Query hooks, types
│   ├── src/stores/               # Zustand auth store
│   └── dist/                    # Production build, embedded into the Go binary
├── migrations/                  # SQL migration files
├── monitoring/                  # Prometheus + Grafana config
├── Dockerfile                   # Node build stage → Go build stage → runtime
├── docker-compose.yml
└── .env.example
```

## Testing

```bash
# All tests (SQLite integration tests run without external services)
go test -short ./...

# With race detector
go test -short -race ./...

# Full integration tests (requires Docker for testcontainers)
go test ./internal/tests/...

# Frontend lint + tests + typecheck + build
cd web && npm run lint && npm test && npm run build
```

## CI/CD

GitHub Actions pipeline on every push to `main` / `develop`:

1. **frontend** — `oxlint`, `vitest`, `tsc`, `vite build`; the built `web/dist` is passed to the next job as an artifact
2. **test** — `golangci-lint`, `go test -race ./...` (with PostgreSQL + Redis service containers), `go build` with the real embedded SPA
3. **docker** — multi-stage image build + push to GHCR (main branch only)

## Security

- JWT access/refresh tokens, refresh rotation on every use, opaque `crypto/rand` refresh tokens (not JWTs, so no collision risk)
- Bcrypt password hashing
- Bans take effect immediately — checked on every request, not just at token issuance
- Role-gated admin/moderator middleware, self-demote and self-ban blocked
- Password reset never reveals whether an email exists; resets revoke all outstanding refresh tokens
- OAuth state-cookie CSRF protection on the GitHub login flow
- Magic-byte file type validation on uploads, server-side size enforcement
- SQL injection prevention via prepared statements / parameterized queries
- Destructive DB operations wrapped in transactions
- Rate limiting per IP (Redis-backed, in-memory fallback)
- Input validation on all endpoints
- Markdown rendered client-side with `react-markdown` (no `dangerouslySetInnerHTML`)

## License

MIT License — see [LICENSE](LICENSE)

## Author

RoomiestThunder
