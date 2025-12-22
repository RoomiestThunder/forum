# Forum Application - Architecture Documentation

## Project Overview

Forum is a web-based discussion platform built with Go, featuring user authentication, post management, and community interaction through comments and voting systems.

## Architecture Principles

### 1. Separation of Concerns

The codebase is organized into distinct packages, each responsible for a specific domain:

- **cmd/forum** - Application bootstrap and configuration loading
- **internal/config** - Configuration management and validation
- **internal/handlers** - HTTP request handling and business logic
- **internal/middleware** - Request processing and cross-cutting concerns
- **internal/database** - Data persistence and database operations
- **internal/errors** - Error handling and custom error types

### 2. Layered Architecture

```
┌─────────────────────────────────────┐
│      HTTP Handlers Layer            │
│   (handlers.go, types.go)           │
├─────────────────────────────────────┤
│    Middleware & Error Handling      │
│   (middleware.go, errors.go)        │
├─────────────────────────────────────┤
│      Business Logic Layer           │
│   (Config, Sessions, Validation)    │
├─────────────────────────────────────┤
│      Data Persistence Layer         │
│   (Store Interface, SQLiteStore)    │
└─────────────────────────────────────┘
```

## Design Patterns

### 1. Repository Pattern

The `Store` interface abstracts database operations:

```go
type Store interface {
    CreateUser(email, username, passwordHash string) (int, error)
    GetUserByEmail(email string) (*User, error)
    CreatePost(userID int, title, content string) (int, error)
    GetPost(id int) (*Post, error)
    // ... 25+ more methods
}
```

**Benefits:**
- Testability with mock implementations
- Database-agnostic operations
- Clear contracts for data operations

### 2. Middleware Chain Pattern

Composable middleware for request processing:

```go
type Handler func(http.ResponseWriter, *http.Request) error
type Middleware func(Handler) Handler

func Chain(h Handler, middlewares ...Middleware) http.Handler {
    for i := len(middlewares) - 1; i >= 0; i-- {
        h = middlewares[i](h)
    }
    return h
}
```

**Implemented Middleware:**
- RecoveryMiddleware - panic recovery
- LoggingMiddleware - structured request logging
- AuthMiddleware - required authentication
- RateLimitMiddleware - per-IP rate limiting
- SecurityHeadersMiddleware - HSTS, CSP, X-Frame-Options
- RequestIDMiddleware - request tracking

### 3. Custom Error Types

Type-safe error handling with HTTP status mapping:

```go
type AppError struct {
    Code    int
    Message string
    Err     error
    Details string
}

func (e *AppError) HTTPStatusCode() int {
    if e.Code >= 100 && e.Code < 600 {
        return e.Code
    }
    return http.StatusInternalServerError
}
```

**Common Error Builders:**
- ErrNotFound - 404
- ErrUnauthorized - 401
- ErrForbidden - 403
- ErrBadRequest - 400
- ErrConflict - 409
- ErrInternalServer - 500
- ErrTooManyRequests - 429

### 4. Configuration Management

Type-safe configuration with validation:

```go
type Config struct {
    Server   ServerConfig
    Database DatabaseConfig
    Session  SessionConfig
    UI       UIConfig
}

func (c *Config) Validate() error {
    // Comprehensive validation logic
}
```

**Features:**
- Environment variable driven (12-factor app)
- Default values
- Comprehensive validation
- Type-safe access

### 5. Dependency Injection

Configuration passed as parameter (no global state):

```go
func StartServer(cfg *config.Config) error {
    // Uses cfg for all configuration needs
}

func (s *SQLiteStore) GetUserByEmail(email string) (*User, error) {
    // Self-contained database operations
}
```

## Data Model

### Entity Relationship Diagram

```
users (1) ──── (N) posts
         ├──────────────(N) comments
         ├──────────────(N) post_likes
         └──────────────(N) comment_likes

posts (1) ────── (N) post_categories
categories (1) ──

comments (1) ─────── (N) comment_likes
```

### Database Tables

| Table | Purpose | Key Fields |
|-------|---------|-----------|
| users | User accounts | id, email, username, password_hash |
| posts | Forum posts | id, user_id, title, content, created_at |
| comments | Post comments | id, post_id, user_id, content, created_at |
| categories | Post categories | id, name |
| post_categories | Post-Category mapping | post_id, category_id |
| post_likes | Post votes | user_id, post_id, is_like |
| comment_likes | Comment votes | user_id, comment_id, is_like |
| sessions | User sessions | id, user_id, uuid, expires |

## Security Architecture

### Authentication & Authorization

1. **Registration** - Email and username uniqueness validation
2. **Login** - Bcrypt password verification
3. **Sessions** - UUID-based tokens with 24-hour expiration
4. **Authorization** - Owner-only access for edit/delete operations

### Security Controls

- **Input Validation** - All user inputs validated before processing
- **SQL Injection Prevention** - Prepared statements with parameterized queries
- **XSS Protection** - Template auto-escaping via html/template
- **CSRF Protection** - Form method validation
- **Rate Limiting** - Token bucket algorithm per IP
- **Password Security** - Bcrypt hashing (cost: 12)
- **Session Management** - Single active session per user

## API Endpoint Structure

### Authentication Endpoints
- `GET/POST /register` - User registration
- `GET/POST /login` - User authentication
- `GET /logout` - Session termination

### Post Management
- `GET /` - Homepage with pagination
- `GET/POST /create_post` - Create new post
- `GET /post/{id}` - View post with comments
- `GET/PUT /edit_post?id={id}` - Edit post
- `DELETE /delete_post?id={id}` - Delete post
- `GET /like_post?id={id}&like=1/0` - Vote on post

### Comment Management
- `POST /post/{id}` - Add comment to post
- `GET/PUT /edit_comment?id={id}&post={post_id}` - Edit comment
- `DELETE /delete_comment?id={id}&post={post_id}` - Delete comment
- `GET /like_comment?id={id}&post={post_id}&like=1/0` - Vote on comment

### Filtering & Pagination
- `GET /?category={id}` - Filter by category
- `GET /?filter=myposts` - User's own posts
- `GET /?filter=liked` - User's liked posts
- `GET /?page={num}` - Pagination

## Testing Strategy

### Test Coverage

| Package | Tests | Type |
|---------|-------|------|
| config | 13 | Unit tests for configuration |
| errors | 14 | Unit tests for error handling |
| middleware | 5 | Unit tests for middleware |
| handlers | 20 | Integration tests for endpoints |
| database | 18 | Integration tests for persistence |
| **Total** | **70** | |

### Test Types

1. **Unit Tests** - Individual component testing
   - Configuration validation
   - Error type behavior
   - Utility functions

2. **Integration Tests** - End-to-end functionality
   - HTTP endpoint behavior
   - Database operations
   - Error handling in context

3. **Edge Cases**
   - Missing parameters
   - Invalid input
   - Non-existent resources
   - Unauthorized access

## Performance Considerations

### Optimization Strategies

1. **Database Connection Pooling**
   - Max open connections: 25
   - Max idle connections: 5
   - Connection max lifetime: 5 minutes

2. **Pagination**
   - Default page size: 5 posts
   - Configurable pagination size
   - Efficient LIMIT/OFFSET queries

3. **Rate Limiting**
   - Token bucket algorithm
   - Per-IP tracking
   - Configurable request limits

4. **Caching**
   - Server-side rendered templates
   - HTTP cache headers for static assets
   - Session caching via in-memory storage

## Deployment Architecture

### Docker Support

Multi-stage build for optimized image:

```dockerfile
FROM golang:1.24.2-alpine AS builder
# Build stage

FROM alpine:latest
# Runtime stage with minimal footprint
```

### Configuration for Production

- Environment-based configuration
- SQLite database (single-file deployment)
- Docker Compose orchestration
- Health checks and container restart policies

## Code Quality Metrics

- **Test Coverage**: 70+ tests, all passing
- **Code Style**: go fmt compliant
- **Static Analysis**: go vet clean
- **Code Organization**: 10 Go files, clear separation
- **Documentation**: Inline comments, API documentation

## Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| github.com/mattn/go-sqlite3 | v1.14.29 | SQLite driver |
| golang.org/x/crypto | v0.40.0 | Bcrypt hashing |
| github.com/google/uuid | v1.6.0 | Session token generation |

## Future Enhancements

1. **Features**
   - Direct messaging between users
   - Post search and advanced filtering
   - User profile customization
   - Admin dashboard

2. **Technical**
   - GraphQL API endpoint
   - WebSocket for real-time updates
   - Full-text search (FTS5)
   - Horizontal scaling with distributed sessions

3. **Operations**
   - Metrics and monitoring (Prometheus)
   - Structured logging (JSON format)
   - Database migrations system
   - CI/CD pipeline automation

## Conclusion

The Forum application follows Go best practices with clear separation of concerns, testable components, and secure design patterns. The modular architecture allows for easy extension and maintenance while maintaining code quality and security.
