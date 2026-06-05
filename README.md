# Forum Application

Production-ready web forum built in Go with JWT authentication, REST API, real-time WebSocket notifications, Redis caching, MinIO file storage, and Prometheus observability.

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go 1.24 |
| Database | PostgreSQL 16 (prod) / SQLite (dev) |
| Cache + Rate limiting | Redis 7 |
| File storage | MinIO (S3-compatible) |
| Auth | JWT (access 15 min + refresh 7 days) + session cookies (HTML) |
| Real-time | WebSocket (gorilla/websocket) |
| Metrics | Prometheus + Grafana |
| CI/CD | GitHub Actions → GHCR |

## Quick Start

### Full stack with Docker Compose

```bash
docker-compose up --build
```

| Service | URL |
|---|---|
| Forum | http://localhost:8080 |
| API | http://localhost:8080/api/v1 |
| Grafana | http://localhost:3000 (admin/admin) |
| Prometheus | http://localhost:9090 |
| MinIO console | http://localhost:9001 (minioadmin/minioadmin) |

### Local development (SQLite, no external services)

```bash
go mod download
CGO_ENABLED=1 go build -o forum ./cmd/forum
./forum
# Visit http://localhost:8080
```

## REST API

All endpoints return JSON. Authenticated endpoints require `Authorization: Bearer <access_token>`.

### Auth

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/auth/register` | — | Register, returns token pair |
| POST | `/api/v1/auth/login` | — | Login, returns token pair |
| POST | `/api/v1/auth/refresh` | — | Rotate refresh token |
| POST | `/api/v1/auth/logout` | ✓ | Revoke refresh tokens |

### Posts

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/posts` | — | List posts (paginated) |
| POST | `/api/v1/posts` | ✓ | Create post |
| GET | `/api/v1/posts/{id}` | — | Get post + comments |
| PUT | `/api/v1/posts/{id}` | ✓ | Update post (owner only) |
| DELETE | `/api/v1/posts/{id}` | ✓ | Delete post (owner only) |
| POST | `/api/v1/posts/{id}` | ✓ | Add comment |

### Other

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/search?q=...` | — | Full-text search (PostgreSQL tsvector / SQLite LIKE) |
| GET | `/api/v1/categories` | — | List categories |
| POST | `/api/v1/like/post` | ✓ | Toggle post like/dislike |
| POST | `/api/v1/like/comment` | ✓ | Toggle comment like/dislike |
| POST | `/api/v1/upload` | ✓ | Upload file (multipart, max 5 MB) |
| GET | `/api/v1/ws?token=...` | ✓ | WebSocket notifications |
| GET | `/api/v1/health` | — | Health check |
| GET | `/metrics` | — | Prometheus metrics |

### Query parameters for `GET /api/v1/posts`

| Parameter | Values | Description |
|---|---|---|
| `page` | integer | Pagination |
| `filter` | `myposts`, `liked` | Filter by ownership / liked |
| `category` | category id | Filter by category |

## HTML Frontend

The classic server-rendered UI remains fully functional alongside the REST API.

- `GET/POST /register` — registration
- `GET/POST /login` — login
- `GET /logout` — logout
- `GET /` — homepage with filtering and pagination
- `GET /post/{id}` — post detail with comments
- `GET/PUT /edit_post?id={id}` — edit post
- `DELETE /delete_post?id={id}` — delete post
- `GET /like_post?id={id}&like=1/0` — like/dislike post
- `POST /post/{id}` — add comment
- `GET/PUT /edit_comment?id={id}&post={post_id}` — edit comment
- `DELETE /delete_comment?id={id}&post={post_id}` — delete comment

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

# Redis
REDIS_URL=redis://localhost:6379

# MinIO / S3
MINIO_ENDPOINT=localhost:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=forum
```

Redis and MinIO are optional — the app starts without them (uploads and cache disabled).

## Database

### PostgreSQL (production)

Schema is applied automatically via `migrations/001_initial.up.sql` when the container starts.
Full-text search uses `tsvector GENERATED ALWAYS` with a GIN index.

### SQLite (development)

Schema is applied automatically on first run. Full-text search falls back to `LIKE`.

## Project Structure

```
forum/
├── cmd/forum/main.go           # Wires all components, starts server
├── internal/
│   ├── models/                 # Shared domain types
│   ├── config/                 # Config from environment variables
│   ├── database/               # Store interface + SQLite + PostgreSQL
│   ├── auth/                   # JWT token manager
│   ├── api/                    # REST API v1 handlers
│   ├── handlers/               # HTML template handlers
│   ├── websocket/              # WebSocket hub (per-user broadcast)
│   ├── cache/                  # Redis cache-aside + rate limiting
│   ├── storage/                # MinIO/S3 file upload
│   ├── metrics/                # Prometheus metrics
│   ├── middleware/             # Logging, recovery, rate limiting
│   ├── errors/                 # Typed application errors
│   └── tests/                  # Integration tests
├── migrations/                 # SQL migration files
├── monitoring/                 # Prometheus + Grafana config
├── templates/                  # HTML templates
├── static/                     # CSS and favicon
├── Dockerfile
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
```

## CI/CD

GitHub Actions pipeline on every push to `main` / `develop`:

1. `golangci-lint`
2. `go test -race ./...` (with PostgreSQL + Redis service containers)
3. `go build`
4. Docker build + push to GHCR (main branch only)

## Security

- JWT Bearer tokens for REST API (access + refresh with rotation)
- Session cookies for HTML frontend (single active session per user)
- Bcrypt password hashing
- Same-origin Referer validation (no open redirect)
- Magic-byte file type validation on uploads
- Server-side file size enforcement (not trusting client headers)
- SQL injection prevention via prepared statements / parameterized queries
- DeletePost wrapped in a transaction (no orphaned rows)
- Input validation on all endpoints

## License

MIT License — see [LICENSE](LICENSE)

## Author

RoomiestThunder
