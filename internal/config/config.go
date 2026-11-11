package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	// Environment is APP_ENV. "production" turns configuration warnings into
	// startup errors.
	Environment string
	Server      ServerConfig
	Database    DatabaseConfig
	JWT         JWTConfig
	Redis       RedisConfig
	MinIO       MinIOConfig
	SMTP        SMTPConfig
	GitHub      GitHubConfig
	BaseURL     string
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
	Secret string
	// SecretGenerated is true when JWT_SECRET wasn't set and Secret is a random
	// value made at startup. That is safe, but every restart invalidates all
	// issued tokens, and several instances would disagree about each other's.
	SecretGenerated      bool
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration
}

const (
	// jwtSecretPlaceholder is the value shipped in .env.example and
	// docker-compose.yml. It is public, so a server signing with it lets anyone
	// forge a token for any user, admins included.
	jwtSecretPlaceholder = "change-me-in-production"
	// minJWTSecretLength is the shortest secret considered strong enough for
	// HS256, whose key should be at least as long as its 256-bit output.
	minJWTSecretLength = 32
)

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
	// PublicURL is where browsers fetch uploaded objects, e.g.
	// "http://localhost:9000/forum". Empty means "derive it from the endpoint",
	// which is wrong whenever the endpoint is an internal name like minio:9000.
	PublicURL string
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
	secret := getEnv("JWT_SECRET", "")
	secretGenerated := secret == ""
	if secretGenerated {
		secret = randomSecret()
	}

	return &Config{
		Environment: getEnv("APP_ENV", ""),
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
			Secret:               secret,
			SecretGenerated:      secretGenerated,
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
			PublicURL:       getEnv("MINIO_PUBLIC_URL", ""),
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
	if c.IsProduction() {
		if problem := c.jwtSecretProblem(); problem != "" {
			return fmt.Errorf("refusing to start in production: %s", problem)
		}
	}
	return nil
}

// IsProduction reports whether APP_ENV=production.
func (c *Config) IsProduction() bool { return c.Environment == "production" }

// Warnings lists configuration that works but is probably not what was meant.
// In production the same problems are errors from Validate.
func (c *Config) Warnings() []string {
	var warnings []string
	if problem := c.jwtSecretProblem(); problem != "" {
		warnings = append(warnings, problem)
	}
	return warnings
}

func (c *Config) jwtSecretProblem() string {
	switch {
	case c.JWT.SecretGenerated:
		return "JWT_SECRET is not set, so a random one was generated: all sessions end whenever the server restarts"
	case c.JWT.Secret == jwtSecretPlaceholder:
		return "JWT_SECRET is the public placeholder value; anyone can forge a login token with it. Set it to a random value of at least 32 characters"
	case len(c.JWT.Secret) < minJWTSecretLength:
		return fmt.Sprintf("JWT_SECRET is shorter than %d characters", minJWTSecretLength)
	}
	return ""
}

// randomSecret returns 32 random bytes, hex encoded.
func randomSecret() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		// No entropy source means no safe way to generate a secret at all.
		panic(fmt.Sprintf("config: cannot generate a JWT secret: %v", err))
	}
	return hex.EncodeToString(buf)
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
