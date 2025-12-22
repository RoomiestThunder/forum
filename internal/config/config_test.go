package config

import (
	"os"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	os.Clearenv()
	cfg := LoadConfig()

	if cfg.Server.Port != 8080 {
		t.Errorf("Expected default port 8080, got %d", cfg.Server.Port)
	}

	if cfg.Database.MaxOpenConn != 25 {
		t.Errorf("Expected default max connections 25, got %d", cfg.Database.MaxOpenConn)
	}

	if cfg.Session.DurationHours != 24 {
		t.Errorf("Expected default session duration 24, got %d", cfg.Session.DurationHours)
	}

	if cfg.UI.PaginationSize != 5 {
		t.Errorf("Expected default pagination size 5, got %d", cfg.UI.PaginationSize)
	}
}

func TestValidateConfigValid(t *testing.T) {
	os.Clearenv()
	cfg := LoadConfig()

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Valid configuration should not produce errors, got: %v", err)
	}
}

func TestValidateConfigInvalidPort(t *testing.T) {
	os.Clearenv()
	os.Setenv("PORT", "99999")

	cfg := LoadConfig()
	err := cfg.Validate()

	if err == nil {
		t.Error("Expected validation error for invalid port, got nil")
	}
}

func TestValidateConfigZeroPort(t *testing.T) {
	os.Clearenv()
	os.Setenv("PORT", "0")

	cfg := LoadConfig()
	err := cfg.Validate()

	if err == nil {
		t.Error("Expected validation error for port 0, got nil")
	}
}

func TestValidateConfigInvalidMaxConnections(t *testing.T) {
	os.Clearenv()
	os.Setenv("DB_MAX_OPEN_CONN", "0")

	cfg := LoadConfig()
	err := cfg.Validate()

	if err == nil {
		t.Error("Expected validation error for 0 max connections, got nil")
	}
}

func TestValidateConfigInvalidPaginationSize(t *testing.T) {
	os.Clearenv()
	os.Setenv("PAGINATION_SIZE", "0")

	cfg := LoadConfig()
	err := cfg.Validate()

	if err == nil {
		t.Error("Expected validation error for pagination size 0, got nil")
	}
}

func TestGetEnvWithDefault(t *testing.T) {
	os.Clearenv()

	os.Setenv("TEST_VAR", "test_value")
	result := getEnv("TEST_VAR", "default")
	if result != "test_value" {
		t.Errorf("Expected 'test_value', got '%s'", result)
	}

	result = getEnv("NONEXISTENT_VAR", "default")
	if result != "default" {
		t.Errorf("Expected 'default', got '%s'", result)
	}
}

func TestGetEnvIntWithDefault(t *testing.T) {
	os.Clearenv()

	os.Setenv("TEST_INT", "42")
	result := getEnvInt("TEST_INT", 0)
	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}

	result = getEnvInt("NONEXISTENT_INT", 77)
	if result != 77 {
		t.Errorf("Expected default value 77, got %d", result)
	}
}

func TestConfigCookieSecurity(t *testing.T) {
	os.Clearenv()
	cfg := LoadConfig()

	if !cfg.Session.HttpOnly {
		t.Error("Expected HttpOnly to be true by default")
	}

	if cfg.Session.CookieSameSite == "" {
		t.Error("Expected CookieSameSite to be set")
	}

	if cfg.Session.CookieName == "" {
		t.Error("Expected CookieName to be set")
	}
}

func TestConfigDatabaseSettings(t *testing.T) {
	os.Clearenv()
	cfg := LoadConfig()

	if cfg.Database.Path == "" {
		t.Error("Expected database path to be set")
	}

	if cfg.Database.MaxOpenConn <= 0 {
		t.Error("Expected positive max connections")
	}

	if cfg.Database.MaxIdleConn < 0 {
		t.Error("Expected non-negative max idle connections")
	}
}

func TestConfigServerTimeouts(t *testing.T) {
	os.Clearenv()
	cfg := LoadConfig()

	if cfg.Server.ReadTimeout <= 0 {
		t.Errorf("Expected positive read timeout, got %d", cfg.Server.ReadTimeout)
	}

	if cfg.Server.WriteTimeout <= 0 {
		t.Errorf("Expected positive write timeout, got %d", cfg.Server.WriteTimeout)
	}

	if cfg.Server.IdleTimeout <= 0 {
		t.Errorf("Expected positive idle timeout, got %d", cfg.Server.IdleTimeout)
	}
}
