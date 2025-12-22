# Forum Application

Web forum built in Go with user authentication, posts, comments, and likes/dislikes system.

## Quick Start

### Docker (Recommended)
```bash
docker-compose up --build
```
Visit: http://localhost:8080

### Local Development
```bash
go mod download
CGO_ENABLED=1 go build -o forum
./forum
```

## Features

- User registration and authentication (email/username login)
- Create, edit, delete posts with categories
- Comment system with full CRUD
- Like/dislike voting for posts and comments
- Category-based filtering
- Pagination
- User session management (24-hour tokens)
- Password hashing with bcrypt
- SQLite database

## Tech Stack

- **Backend:** Go 1.24.2 (net/http, html/template, database/sql)
- **Database:** SQLite3
- **Frontend:** Bootstrap 5, Server-side rendering
- **Security:** Bcrypt password hashing, UUID sessions
- **Dependencies:**
  - github.com/mattn/go-sqlite3
  - golang.org/x/crypto/bcrypt
  - github.com/google/uuid

## Configuration

Environment variables (see `.env.example`):
- `PORT` - Server port (default: 8080)
- `DB_PATH` - Database file path (default: forum.db)
- `SESSION_DURATION_HOURS` - Session timeout (default: 24)
- `PAGINATION_SIZE` - Posts per page (default: 5)

## API Endpoints

### Authentication
- `GET/POST /register` - User registration
- `GET/POST /login` - User login
- `GET /logout` - User logout

### Posts
- `GET /` - Forum homepage with pagination
- `GET /create_post` - Create post form
- `POST /create_post` - Create post
- `GET /post/{id}` - View post with comments
- `GET/PUT /edit_post?id={id}` - Edit post
- `DELETE /delete_post?id={id}` - Delete post
- `GET /like_post?id={id}&like=1/0` - Like/dislike post

### Comments
- `POST /post/{id}` - Add comment
- `GET/PUT /edit_comment?id={id}&post={post_id}` - Edit comment
- `DELETE /delete_comment?id={id}&post={post_id}` - Delete comment
- `GET /like_comment?id={id}&post={post_id}&like=1/0` - Like/dislike comment

### Filtering
- `GET /?category={id}` - Filter by category
- `GET /?filter=myposts` - User's posts
- `GET /?filter=liked` - User's liked posts
- `GET /?page={num}` - Pagination

## Database Schema

- **users** - User accounts (email, username, bcrypt password)
- **posts** - Forum posts (title, content, user_id, created_at)
- **comments** - Post comments (content, post_id, user_id, created_at)
- **categories** - Post categories
- **post_categories** - Many-to-many relationship
- **post_likes** - Post votes (user_id, post_id, is_like)
- **comment_likes** - Comment votes (user_id, comment_id, is_like)
- **sessions** - User sessions (uuid, user_id, expires)

## Security

- Bcrypt password hashing (cost: 12)
- UUID-based sessions with 24-hour expiration
- Single active session per user
- SQL injection prevention via prepared statements
- XSS protection via template auto-escaping
- HTTP method enforcement
- Owner-only access to edit/delete operations
- Input validation on all endpoints

## Files

```
forum/
├── cmd/
│   └── forum/
│       └── main.go                 # Application entry point (30 lines)
├── internal/
│   ├── config/
│   │   ├── config.go               # Configuration management (151 lines)
│   │   └── config_test.go          # Config tests (13 tests)
│   ├── database/
│   │   └── database.go             # Store interface & SQLite impl (539 lines)
│   ├── errors/
│   │   ├── errors.go               # Custom error types (106 lines)
│   │   └── errors_test.go          # Error tests (14 tests)
│   ├── handlers/
│   │   ├── handlers.go             # HTTP handlers (1190 lines)
│   │   └── types.go                # Domain types (43 lines)
│   └── middleware/
│       ├── middleware.go           # Middleware chain (265 lines)
│       └── middleware_test.go      # Middleware tests (5 tests)
├── templates/                      # HTML templates (9 files)
├── static/                         # CSS and favicon
├── go.mod/go.sum                   # Go dependencies
├── Dockerfile                      # Multi-stage Docker build
├── docker-compose.yml              # Docker orchestration
├── .env.example                    # Environment template
├── .gitignore                      # Git ignore rules
├── LICENSE                         # MIT License
└── README.md                       # This file
```

## Project Structure

**cmd/** - Application entry points
- `forum/main.go` - Server startup and configuration loading

**internal/** - Private packages (not importable from outside)
- `config/` - Configuration management and validation
- `database/` - Data persistence layer with Store interface
- `errors/` - Custom error types with HTTP status mapping
- `handlers/` - HTTP request handlers and business logic
- `middleware/` - Request processing middleware chain

**templates/** - Server-side rendered HTML templates
**static/** - CSS stylesheets and favicon

## Building & Running

### With Docker Compose (Recommended)
```bash
docker-compose up --build
# Visit http://localhost:8080
```

### Manual Build & Run
```bash
# Download dependencies
go mod download

# Build binary (requires CGO for SQLite)
CGO_ENABLED=1 go build -o forum ./cmd/forum

# Run server
./forum
```

### Run Tests
```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific package tests
go test -v ./internal/config
go test -v ./internal/errors
go test -v ./internal/middleware
```

### Code Quality
```bash
# Format code
go fmt ./...

# Analyze code
go vet ./...
```

## Environment Configuration

Create `.env` file from template:
```bash
cp .env.example .env
```

Configuration options:
- `PORT` - Server port (default: 8080)
- `DB_PATH` - SQLite database path (default: forum.db)
- `DB_MAX_OPEN_CONN` - Connection pool size (default: 25)
- `SESSION_DURATION_HOURS` - Session timeout (default: 24)
- `PAGINATION_SIZE` - Posts per page (default: 5)
- `READ_TIMEOUT_SECS` - HTTP read timeout (default: 15s)
- `WRITE_TIMEOUT_SECS` - HTTP write timeout (default: 15s)

## License

MIT License - See LICENSE file

## Author

RoomiestThunder
