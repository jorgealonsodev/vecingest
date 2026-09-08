package config_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/config"
)

func validEncryptionKey() string {
	return base64.StdEncoding.EncodeToString(make([]byte, 32))
}

// fullValidEnv returns a superset of every variable any M0 subcommand
// might require, so each test case starts from a known-good baseline and
// removes exactly the variable under test.
func fullValidEnv() map[string]string {
	return map[string]string{ //nolint:gosec // G101: fake fixture DSNs/passwords for a test-only local environment map, never real credentials
		"JWT_SECRET":              "test-jwt-secret",
		"JWT_REFRESH_SECRET":      "test-jwt-refresh-secret",
		"DOMAIN":                  "example.com",
		"DATABASE_URL":            "postgres://app_rw:pw@localhost:5432/vecingest",
		"PROXY_IP":                "172.18.0.2",
		"CORS_ORIGINS":            "https://app.example.com",
		"ENCRYPTION_KEY":          validEncryptionKey(),
		"SMTP_URL":                "smtps://user:pw@smtp.example.com:465",
		"MAIL_FROM":               "Vecingest <no-reply@example.com>",
		"APP_ENV":                 "production",
		"PORT":                    "3000",
		"BOOTSTRAP_DATABASE_URL":  "postgres://postgres:pw@localhost:5432/vecingest",
		"MIGRATIONS_DATABASE_URL": "postgres://vecingest_owner:pw@localhost:5432/vecingest",
		"APP_DB_USER":             "app_rw",
		"APP_DB_PASSWORD":         "pw",
	}
}

// setEnv sets every var in vars via t.Setenv (auto-restored at test end),
// then explicitly unsets everything named in omit so the negative test
// cases exercise a truly absent variable rather than an empty one.
func setEnv(t *testing.T, vars map[string]string, omit ...string) {
	t.Helper()
	skip := make(map[string]bool, len(omit))
	for _, name := range omit {
		skip[name] = true
	}
	for k, v := range vars {
		if skip[k] {
			continue
		}
		t.Setenv(k, v)
	}
	for _, name := range omit {
		os.Unsetenv(name) //nolint:errcheck // best-effort cleanup of a var that may not be set
	}
}

func TestLoad_PerSubcommandRequiredSet(t *testing.T) {
	tests := []struct {
		name    string
		cmd     config.Command
		missing string // variable to omit; "" means load the full valid set
	}{
		{"serve full set passes", config.CommandServe, ""},
		{"serve missing JWT_SECRET", config.CommandServe, "JWT_SECRET"},
		{"serve missing DATABASE_URL", config.CommandServe, "DATABASE_URL"},
		{"serve missing CORS_ORIGINS", config.CommandServe, "CORS_ORIGINS"},
		{"serve missing ENCRYPTION_KEY", config.CommandServe, "ENCRYPTION_KEY"},
		{"serve missing DOMAIN", config.CommandServe, "DOMAIN"},
		{"migrate full set passes", config.CommandMigrate, ""},
		{"migrate missing BOOTSTRAP_DATABASE_URL", config.CommandMigrate, "BOOTSTRAP_DATABASE_URL"},
		{"migrate missing APP_DB_PASSWORD", config.CommandMigrate, "APP_DB_PASSWORD"},
		{"worker full set passes (no JWT/PROXY_IP/CORS/PORT/SMTP required)", config.CommandWorker, ""},
		{"worker missing DATABASE_URL", config.CommandWorker, "DATABASE_URL"},
		{"seed full set passes (no ENCRYPTION_KEY required)", config.CommandSeed, ""},
		{"seed missing DATABASE_URL", config.CommandSeed, "DATABASE_URL"},
		{"bootstrap-superadmin full set passes (no APP_ENV required)", config.CommandBootstrapSuperadmin, ""},
		{"bootstrap-superadmin missing ENCRYPTION_KEY", config.CommandBootstrapSuperadmin, "ENCRYPTION_KEY"},
		{"health-ready full set passes", config.CommandHealthReady, ""},
		{"health-ready missing PORT", config.CommandHealthReady, "PORT"},
		{"health-worker full set passes", config.CommandHealthWorker, ""},
		{"health-worker missing DATABASE_URL", config.CommandHealthWorker, "DATABASE_URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var omit []string
			if tt.missing != "" {
				omit = append(omit, tt.missing)
			}
			setEnv(t, fullValidEnv(), omit...)

			_, holder, err := config.Load(context.Background(), os.LookupEnv, tt.cmd)

			if tt.missing == "" {
				if err != nil {
					t.Fatalf("expected the full required set to pass for %s, got error: %v", tt.cmd, err)
				}
				if holder == nil {
					t.Fatalf("expected a non-nil secrets holder on success")
				}
				return
			}

			if err == nil {
				t.Fatalf("expected Load to fail when %s is missing for %s", tt.missing, tt.cmd)
			}
			if !strings.Contains(err.Error(), tt.missing) {
				t.Fatalf("expected error to name the missing variable %s, got: %v", tt.missing, err)
			}
		})
	}
}

// platform-bootstrap: Config Startup Validation -- DOMAIN is a required,
// non-secret serve variable (design D-I: Config "carries DOMAIN, ...").
// serve.go derives the refresh cookie's Domain=api.DOMAIN (D-E) and
// CORS's own credentialed-origin allowlist from it.
func TestLoad_DomainExposedOnConfig(t *testing.T) {
	setEnv(t, fullValidEnv())
	cfg, _, err := config.Load(context.Background(), os.LookupEnv, config.CommandServe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Domain != "example.com" {
		t.Fatalf("expected cfg.Domain to be %q, got %q", "example.com", cfg.Domain)
	}
}

func TestLoad_UnknownCommandRejected(t *testing.T) {
	setEnv(t, fullValidEnv())
	_, _, err := config.Load(context.Background(), os.LookupEnv, config.Command("not-a-real-command"))
	if err == nil {
		t.Fatalf("expected an unknown command to be rejected")
	}
}

func TestLoad_ProxyIPRequiredUnlessDevelopment(t *testing.T) {
	t.Run("staging requires PROXY_IP", func(t *testing.T) {
		env := fullValidEnv()
		env["APP_ENV"] = "staging"
		setEnv(t, env, "PROXY_IP")
		_, _, err := config.Load(context.Background(), os.LookupEnv, config.CommandServe)
		if err == nil || !strings.Contains(err.Error(), "PROXY_IP") {
			t.Fatalf("expected PROXY_IP to be required under APP_ENV=staging, got: %v", err)
		}
	})

	t.Run("development allows PROXY_IP to be unset", func(t *testing.T) {
		env := fullValidEnv()
		env["APP_ENV"] = "development"
		setEnv(t, env, "PROXY_IP")
		_, _, err := config.Load(context.Background(), os.LookupEnv, config.CommandServe)
		if err != nil {
			t.Fatalf("expected APP_ENV=development to allow an unset PROXY_IP, got: %v", err)
		}
	})
}

func TestLoad_InvalidAppEnvRejected(t *testing.T) {
	env := fullValidEnv()
	env["APP_ENV"] = "banana"
	setEnv(t, env)
	_, _, err := config.Load(context.Background(), os.LookupEnv, config.CommandServe)
	if err == nil || !strings.Contains(err.Error(), "APP_ENV") {
		t.Fatalf("expected an invalid APP_ENV value to be rejected by name, got: %v", err)
	}
}

func TestLoad_EncryptionKeyIsolation(t *testing.T) {
	setEnv(t, fullValidEnv())

	cfg, holder, err := config.Load(context.Background(), os.LookupEnv, config.CommandServe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if v := os.Getenv("ENCRYPTION_KEY"); v != "" {
		t.Fatalf("expected ENCRYPTION_KEY to be unset from the process environment after Load, got %q", v)
	}

	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	lower := strings.ToLower(string(cfgJSON))
	if strings.Contains(lower, "encryption_key") || strings.Contains(lower, "encryptionkey") {
		t.Fatalf("expected the serialized config to carry no ENCRYPTION_KEY field, got: %s", cfgJSON)
	}

	if len(holder.EncryptionKey()) != 32 {
		t.Fatalf("expected the holder to carry the 32-byte decoded encryption key, got %d bytes", len(holder.EncryptionKey()))
	}
}

func TestLoad_MalformedEncryptionKeyRejected(t *testing.T) {
	env := fullValidEnv()
	env["ENCRYPTION_KEY"] = "not-base64-and-not-32-bytes"
	setEnv(t, env)
	_, _, err := config.Load(context.Background(), os.LookupEnv, config.CommandServe)
	if err == nil || !strings.Contains(err.Error(), "ENCRYPTION_KEY") {
		t.Fatalf("expected a malformed ENCRYPTION_KEY to be rejected by name, got: %v", err)
	}
}

// platform-bootstrap: Per-Role Database Write and Read Handles --
// "Unconfigured overrides default to DATABASE_URL".
func TestLoad_ReadAndWorkerDSNsDefaultToDatabaseURL(t *testing.T) {
	env := fullValidEnv()
	setEnv(t, env, "DATABASE_URL_READ", "DATABASE_URL_WORKER")

	_, holder, err := config.Load(context.Background(), os.LookupEnv, config.CommandServe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if holder.DatabaseURLRead() != holder.DatabaseURL() {
		t.Fatalf("expected DatabaseURLRead to default to DatabaseURL, got %q vs %q", holder.DatabaseURLRead(), holder.DatabaseURL())
	}

	_, workerHolder, err := config.Load(context.Background(), os.LookupEnv, config.CommandWorker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if workerHolder.DatabaseURLWorker() != workerHolder.DatabaseURL() {
		t.Fatalf("expected DatabaseURLWorker to default to DatabaseURL, got %q vs %q", workerHolder.DatabaseURLWorker(), workerHolder.DatabaseURL())
	}
}

// Triangulation: an explicit override is preserved, not overwritten.
func TestLoad_ConfiguredReadAndWorkerDSNsArePreserved(t *testing.T) {
	env := fullValidEnv()
	env["DATABASE_URL_READ"] = "postgres://app_rw:pw@read-replica:5432/vecingest"
	env["DATABASE_URL_WORKER"] = "postgres://app_rw:pw@worker-node:5432/vecingest"
	setEnv(t, env)

	_, holder, err := config.Load(context.Background(), os.LookupEnv, config.CommandServe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if holder.DatabaseURLRead() != env["DATABASE_URL_READ"] {
		t.Fatalf("expected the configured DATABASE_URL_READ to be preserved, got %q", holder.DatabaseURLRead())
	}
}
