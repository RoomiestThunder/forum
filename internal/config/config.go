package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the application configuration
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Session  SessionConfig
	UI       UIConfig
}

// ServerConfig contains HTTP server configuration
type ServerConfig struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// DatabaseConfig contains database connection configuration
type DatabaseConfig struct {
	Path        string
	MaxOpenConn int
	MaxIdleConn int
	ConnMaxLife time.Duration
}

// SessionConfig contains session management configuration
type SessionConfig struct {
	DurationHours  int
	CookieName     string
	CookiePath     string
	CookieSameSite string
	HttpOnly       bool
	Secure         bool
}

// UIConfig contains user interface configuration
type UIConfig struct {
	PaginationSize int
	TitleMaxLen    int
	CommentMaxLen  int
}

// LoadConfig loads configuration from environment variables with defaults
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
	}
}

// getEnv returns the value of an environment variable, or a default value if not set
func getEnv(key, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultVal
}

// getEnvInt returns the integer value of an environment variable, or a default value if not set
func getEnvInt(key string, defaultVal int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultVal
}

// getEnvBool returns the boolean value of an environment variable, or a default value if not set
func getEnvBool(key string, defaultVal bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultVal
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d (must be between 1 and 65535)", c.Server.Port)
	}

	if c.Database.Path == "" {
		return fmt.Errorf("database path cannot be empty")
	}

	if c.Database.MaxOpenConn < 1 {
		return fmt.Errorf("database max open connections must be at least 1")
	}

	if c.Database.MaxIdleConn < 0 {
		return fmt.Errorf("database max idle connections cannot be negative")
	}

	if c.Session.DurationHours < 1 {
		return fmt.Errorf("session duration must be at least 1 hour")
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

// GetServerAddr returns the server address in format "host:port"
func (c *Config) GetServerAddr() string {
	return fmt.Sprintf(":%d", c.Server.Port)
}
