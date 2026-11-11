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
	JWT      JWTConfig
	Redis    RedisConfig
	MinIO    MinIOConfig
	SMTP     SMTPConfig
	GitHub   GitHubConfig
	BaseURL  string
}

type ServerConfig struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	// TrustProxy makes the app take the client address from X-Forwarded-For /
	// X-Real-IP. Only safe behind a reverse proxy that sets them.
	TrustProxy bool
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

// SMTPConfig configures transactional email. When Host is empty the mailer
// falls back to logging the message instead of sending it, the same
// graceful-degradation pattern used for Redis and MinIO.
type SMTPConfig struct {
	Host string
	Port int
	User string
	Pass string
	From string
}

// GitHubConfig enables "Sign in with GitHub" when both fields are set.
type GitHubConfig struct {
	ClientID     string
	ClientSecret string
}

func LoadConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         getEnvInt("PORT", 8080),
			ReadTimeout:  time.Duration(getEnvInt("READ_TIMEOUT_SECS", 15)) * time.Second,
			WriteTimeout: time.Duration(getEnvInt("WRITE_TIMEOUT_SECS", 15)) * time.Second,
			IdleTimeout:  time.Duration(getEnvInt("IDLE_TIMEOUT_SECS", 60)) * time.Second,
			TrustProxy:   getEnvBool("TRUST_PROXY", false),
		},
		Database: DatabaseConfig{
			Path:        getEnv("DB_PATH", "forum.db"),
			DSN:         getEnv("DATABASE_URL", ""),
			MaxOpenConn: getEnvInt("DB_MAX_OPEN_CONN", 25),
			MaxIdleConn: getEnvInt("DB_MAX_IDLE_CONN", 5),
			ConnMaxLife: time.Duration(getEnvInt("DB_CONN_MAX_LIFE_MINS", 5)) * time.Minute,
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
		SMTP: SMTPConfig{
			Host: getEnv("SMTP_HOST", ""),
			Port: getEnvInt("SMTP_PORT", 1025),
			User: getEnv("SMTP_USER", ""),
			Pass: getEnv("SMTP_PASS", ""),
			From: getEnv("SMTP_FROM", "forum@localhost"),
		},
		GitHub: GitHubConfig{
			ClientID:     getEnv("GITHUB_CLIENT_ID", ""),
			ClientSecret: getEnv("GITHUB_CLIENT_SECRET", ""),
		},
		BaseURL: getEnv("BASE_URL", "http://localhost:8080"),
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
