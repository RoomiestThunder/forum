package config

import (
	"os"
	"strings"
	"testing"
)

// configEnv is every variable LoadConfig reads. clearConfigEnv unsets them so a
// test sees the real defaults whatever the developer's shell or CI exports.
var configEnv = []string{
	"APP_ENV", "PORT", "READ_TIMEOUT_SECS", "WRITE_TIMEOUT_SECS", "IDLE_TIMEOUT_SECS", "TRUST_PROXY",
	"DB_PATH", "DATABASE_URL", "DB_MAX_OPEN_CONN", "DB_MAX_IDLE_CONN", "DB_CONN_MAX_LIFE_MINS",
	"JWT_SECRET", "JWT_ACCESS_DURATION_MIN", "JWT_REFRESH_DURATION_DAYS",
	"REDIS_URL", "REDIS_PASSWORD", "REDIS_DB",
	"MINIO_ENDPOINT", "MINIO_ACCESS_KEY", "MINIO_SECRET_KEY", "MINIO_BUCKET", "MINIO_USE_SSL",
	"SMTP_HOST", "SMTP_PORT", "SMTP_USER", "SMTP_PASS", "SMTP_FROM",
	"GITHUB_CLIENT_ID", "GITHUB_CLIENT_SECRET", "BASE_URL",
}

// clearConfigEnv unsets the configuration variables for the test and restores
// them afterwards. (t.Setenv registers the restore; Unsetenv then removes the
// variable, which t.Setenv alone can't do - it can only set it, even to "".)
func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range configEnv {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}

const strongSecret = "0123456789abcdef0123456789abcdef-strong"

func TestLoadConfigDefaults(t *testing.T) {
	clearConfigEnv(t)
	cfg := LoadConfig()

	if cfg.Server.Port != 8080 {
		t.Errorf("default port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Database.MaxOpenConn != 25 {
		t.Errorf("default max open connections = %d, want 25", cfg.Database.MaxOpenConn)
	}
	if cfg.UsePostgres() {
		t.Error("with no DATABASE_URL, SQLite should be used")
	}
	if cfg.Server.TrustProxy {
		t.Error("TRUST_PROXY must default to false: trusting forwarding headers without a proxy is spoofable")
	}
}

func TestEnvironmentOverridesDefaults(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("PORT", "9090")
	t.Setenv("DB_MAX_OPEN_CONN", "7")
	t.Setenv("DATABASE_URL", "postgres://u:p@h/db")
	t.Setenv("BASE_URL", "https://forum.example")

	cfg := LoadConfig()

	if cfg.Server.Port != 9090 || cfg.Database.MaxOpenConn != 7 || cfg.BaseURL != "https://forum.example" {
		t.Errorf("overrides not applied: %+v", cfg)
	}
	if !cfg.UsePostgres() {
		t.Error("DATABASE_URL should select PostgreSQL")
	}
	if cfg.GetServerAddr() != ":9090" {
		t.Errorf("GetServerAddr = %q", cfg.GetServerAddr())
	}
}

func TestUnparsableNumbersFallBackToDefaults(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("PORT", "not-a-number")

	if got := LoadConfig().Server.Port; got != 8080 {
		t.Errorf("port = %d, want the default 8080", got)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"defaults are valid", nil, ""},
		{"port too high", map[string]string{"PORT": "99999"}, "invalid server port"},
		{"port zero", map[string]string{"PORT": "0"}, "invalid server port"},
		{"no open connections", map[string]string{"DB_MAX_OPEN_CONN": "0"}, "at least 1"},
		{"negative idle connections", map[string]string{"DB_MAX_IDLE_CONN": "-1"}, "negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			err := LoadConfig().Validate()
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("error = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestServerTimeoutsAreSet(t *testing.T) {
	clearConfigEnv(t)
	cfg := LoadConfig()

	if cfg.Server.ReadTimeout <= 0 || cfg.Server.WriteTimeout <= 0 || cfg.Server.IdleTimeout <= 0 {
		t.Errorf("timeouts must be positive: %+v", cfg.Server)
	}
}

func TestTrustProxy(t *testing.T) {
	for value, want := range map[string]bool{"true": true, "1": true, "false": false, "maybe": false} {
		t.Run("TRUST_PROXY="+value, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("TRUST_PROXY", value)
			if got := LoadConfig().Server.TrustProxy; got != want {
				t.Fatalf("TRUST_PROXY=%q: got %v, want %v (an unparsable value must not enable trust)", value, got, want)
			}
		})
	}
}

func TestMissingJWTSecretIsReplacedByARandomOne(t *testing.T) {
	clearConfigEnv(t)

	a, b := LoadConfig(), LoadConfig()

	if !a.JWT.SecretGenerated || a.JWT.Secret == "" {
		t.Fatalf("an unset JWT_SECRET should produce a generated secret, got %+v", a.JWT)
	}
	if len(a.JWT.Secret) < minJWTSecretLength {
		t.Errorf("generated secret is only %d characters", len(a.JWT.Secret))
	}
	if a.JWT.Secret == b.JWT.Secret {
		t.Error("each start must generate a different secret, never a fixed default")
	}
	if len(a.Warnings()) == 0 {
		t.Error("running on a generated secret should be flagged, since sessions won't survive a restart")
	}
}

func TestJWTSecretChecks(t *testing.T) {
	tests := []struct {
		name         string
		appEnv       string
		secret       string
		wantWarning  bool
		wantStartErr bool
	}{
		{"strong secret", "", strongSecret, false, false},
		{"strong secret in production", "production", strongSecret, false, false},
		{"placeholder outside production", "", jwtSecretPlaceholder, true, false},
		{"placeholder in production", "production", jwtSecretPlaceholder, true, true},
		{"short secret outside production", "", "short", true, false},
		{"short secret in production", "production", "short", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("JWT_SECRET", tt.secret)
			t.Setenv("APP_ENV", tt.appEnv)
			cfg := LoadConfig()

			if got := len(cfg.Warnings()) > 0; got != tt.wantWarning {
				t.Errorf("warning = %v, want %v (%v)", got, tt.wantWarning, cfg.Warnings())
			}
			err := cfg.Validate()
			if (err != nil) != tt.wantStartErr {
				t.Errorf("Validate error = %v, want error: %v", err, tt.wantStartErr)
			}
		})
	}
}

func TestProductionRefusesAGeneratedSecret(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("APP_ENV", "production")

	err := LoadConfig().Validate()

	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("production without JWT_SECRET must refuse to start, got %v", err)
	}
}
