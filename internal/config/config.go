package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Session  SessionConfig
	UI       UIConfig
	JWT      JWTConfig
	Redis    RedisConfig
	MinIO    MinIOConfig
}

type ServerConfig struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

type DatabaseConfig struct {
	// SQLite (legacy dev)
	Path string
	// PostgreSQL
	DSN         string
	MaxOpenConn int
	MaxIdleConn int
	ConnMaxLife time.Duration
}

type SessionConfig struct {
	DurationHours  int
	CookieName     string
	CookiePath     string
	CookieSameSite string
	HttpOnly       bool
	Secure         bool
}

type UIConfig struct {
	PaginationSize int
	TitleMaxLen    int
	CommentMaxLen  int
}

type JWTConfig struct {
	Secret               string
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration
}

type RedisConfig struct {
	URL      string
	Password string
	DB       int
}

type MinIOConfig struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	BucketName      string
	UseSSL          bool
}

func LoadConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         getEnvInt("PORT", 8080),
			ReadTimeout:  time.Duration(getEnvInt("READ_TIMEOUT_SECS", 15)) * time.Second,
			WriteTimeout: time.Duration(getEnvInt("WRITE_TIMEOUT_SECS", 15)) * time.Second,
			IdleTimeout:  time.Duration(getEnvInt("IDLE_TIMEOUT_SECS", 60)) * time.Second,
		},
		Database: DatabaseConfig{
			Path:        getEnv("DB_PATH", "forum.db"),
			DSN:         getEnv("DATABASE_URL", ""),
			MaxOpenConn: getEnvInt("DB_MAX_OPEN_CONN", 25),
			MaxIdleConn: getEnvInt("DB_MAX_IDLE_CONN", 5),
			ConnMaxLife: time.Duration(getEnvInt("DB_CONN_MAX_LIFE_MINS", 5)) * time.Minute,
		},
		Session: SessionConfig{
			DurationHours:  getEnvInt("SESSION_DURATION_HOURS", 24),
			CookieName:     getEnv("COOKIE_NAME", "session"),
			CookiePath:     getEnv("COOKIE_PATH", "/"),
			CookieSameSite: getEnv("COOKIE_SAMESITE", "Lax"),
			HttpOnly:       getEnvBool("COOKIE_HTTPONLY", true),
			Secure:         getEnvBool("COOKIE_SECURE", false),
		},
		UI: UIConfig{
			PaginationSize: getEnvInt("PAGINATION_SIZE", 5),
			TitleMaxLen:    getEnvInt("POST_TITLE_MAX_LEN", 20),
			CommentMaxLen:  getEnvInt("COMMENT_MAX_LEN", 500),
		},
		JWT: JWTConfig{
			Secret:               getEnv("JWT_SECRET", "change-me-in-production"),
			AccessTokenDuration:  time.Duration(getEnvInt("JWT_ACCESS_DURATION_MIN", 15)) * time.Minute,
			RefreshTokenDuration: time.Duration(getEnvInt("JWT_REFRESH_DURATION_DAYS", 7)) * 24 * time.Hour,
		},
		Redis: RedisConfig{
			URL:      getEnv("REDIS_URL", "redis://localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		MinIO: MinIOConfig{
			Endpoint:        getEnv("MINIO_ENDPOINT", "localhost:9000"),
			AccessKeyID:     getEnv("MINIO_ACCESS_KEY", "minioadmin"),
			SecretAccessKey: getEnv("MINIO_SECRET_KEY", "minioadmin"),
			BucketName:      getEnv("MINIO_BUCKET", "forum"),
			UseSSL:          getEnvBool("MINIO_USE_SSL", false),
		},
	}
}

func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	if c.Database.DSN == "" && c.Database.Path == "" {
		return fmt.Errorf("either DATABASE_URL or DB_PATH must be set")
	}
	if c.Database.MaxOpenConn < 1 {
		return fmt.Errorf("database max open connections must be at least 1")
	}
	if c.Database.MaxIdleConn < 0 {
		return fmt.Errorf("database max idle connections cannot be negative")
	}
	if c.UI.PaginationSize < 1 {
		return fmt.Errorf("pagination size must be at least 1")
	}
	if c.UI.TitleMaxLen < 1 {
		return fmt.Errorf("title max length must be at least 1")
	}
	if c.UI.CommentMaxLen < 1 {
		return fmt.Errorf("comment max length must be at least 1")
	}
	return nil
}

func (c *Config) GetServerAddr() string {
	return fmt.Sprintf(":%d", c.Server.Port)
}

func (c *Config) UsePostgres() bool {
	return c.Database.DSN != ""
}

func getEnv(key, defaultVal string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if v, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return defaultVal
}
