// Package config implements startup environment-variable validation for
// every vecingest subcommand (design D-I). Validation is fail-fast and
// total: every missing or malformed variable is collected and reported
// by name only, never by value, before the process binds a listener or
// opens a database connection.
package config

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/jorgealonsodev/vecingest/internal/config/secrets"
)

// Command identifies which vecingest subcommand is starting. Every
// subcommand has its own declared requirement set (D-I); a subcommand
// with no declared set would silently inherit serve's, which is how a
// worker ends up demanding a JWT secret it never uses.
type Command string

const (
	CommandServe Command = "serve"
	// CommandMigrate covers both `serve --migrate` and `migrate`: D-I
	// gives them an identical requirement set.
	CommandMigrate             Command = "migrate"
	CommandWorker              Command = "worker"
	CommandSeed                Command = "seed"
	CommandBootstrapSuperadmin Command = "bootstrap-superadmin"
	// CommandHealthReady covers both `health --live` and `health
	// --ready`: both need only PORT (D-I).
	CommandHealthReady  Command = "health-ready"
	CommandHealthWorker Command = "health-worker"
)

// AppEnv is the PRD's own environment-domain variable -- never NODE_ENV.
type AppEnv string

const (
	AppEnvDevelopment AppEnv = "development"
	AppEnvStaging     AppEnv = "staging"
	AppEnvProduction  AppEnv = "production"
)

const (
	envDomain               = "DOMAIN"
	envAppEnv               = "APP_ENV"
	envPort                 = "PORT"
	envProxyIP              = "PROXY_IP"
	envCorsOrigins          = "CORS_ORIGINS"
	envMailFrom             = "MAIL_FROM"
	envSMTPURL              = "SMTP_URL"
	envDatabaseURL          = "DATABASE_URL"
	envDatabaseURLRead      = "DATABASE_URL_READ"
	envDatabaseURLWorker    = "DATABASE_URL_WORKER"
	envBootstrapDatabaseURL = "BOOTSTRAP_DATABASE_URL"
	envAppDBUser            = "APP_DB_USER"
	envAppDBPassword        = "APP_DB_PASSWORD"
	envJWTSecret            = "JWT_SECRET"
	envJWTSecretPrevious    = "JWT_SECRET_PREVIOUS" //nolint:gosec // G101: an environment variable NAME, not a credential value
	envJWTRefreshSecret     = "JWT_REFRESH_SECRET"  //nolint:gosec // G101: an environment variable NAME, not a credential value
	envEncryptionKey        = "ENCRYPTION_KEY"
	envSentryDSN            = "SENTRY_DSN"
	envTurnstileSecret      = "TURNSTILE_SECRET"
)

// Config is the exported, log-safe object. It holds no secret: every
// credential-bearing value lives in the accompanying secrets.Holder.
type Config struct {
	AppEnv AppEnv
	// Domain is the bare apex domain (design D-I; D-E: the refresh
	// cookie's Domain=api.DOMAIN, never DOMAIN itself). Not a secret --
	// it is dictated by DNS, not by anything an attacker learning it
	// could exploit differently than the CORS_ORIGINS entries already
	// disclose.
	Domain       string
	Port         string
	ProxyIP      string
	CorsOrigins  []string
	MailFrom     string
	AppDBUser    string
	DatabaseHost string // host[:port] only, parsed from DATABASE_URL -- never the credentials
}

// LookupEnv matches os.LookupEnv's signature so tests can substitute a
// fake environment. Load always removes ENCRYPTION_KEY from the real
// process environment via os.Unsetenv regardless of the lookup source,
// because platform-bootstrap's ENCRYPTION_KEY Isolation requirement is
// about the real process environment, not about whatever Load was given.
type LookupEnv func(key string) (string, bool)

type invalidVar struct {
	name   string
	reason string // never includes the offending value
}

// ValidationError reports every missing or malformed variable at once,
// by name only, so an operator sees every problem in a single run
// instead of fixing one variable at a time.
type ValidationError struct {
	Missing []string
	Invalid []invalidVar
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Missing)+len(e.Invalid))
	for _, name := range e.Missing {
		parts = append(parts, fmt.Sprintf("%s: missing", name))
	}
	for _, iv := range e.Invalid {
		parts = append(parts, fmt.Sprintf("%s: %s", iv.name, iv.reason))
	}
	sort.Strings(parts)
	return fmt.Sprintf("config validation failed: %s", strings.Join(parts, "; "))
}

// requiredFunc reports whether a variable is required given the current
// environment (e.g. PROXY_IP is required unless APP_ENV=development). A
// nil requiredFunc means the variable is always optional for that command.
type requiredFunc func(lookup LookupEnv) bool

func always(LookupEnv) bool { return true }

func unlessDevelopment(lookup LookupEnv) bool {
	v, _ := lookup(envAppEnv)
	return AppEnv(v) != AppEnvDevelopment
}

// requirementSet returns every variable a command declares, each tagged
// with whether (and when) it is required. A variable absent from the
// returned set is simply not this command's concern -- Load never
// validates or rejects a variable a command did not declare.
func requirementSet(cmd Command) map[string]requiredFunc {
	serveVars := map[string]requiredFunc{
		envDomain:           always,
		envJWTSecret:        always,
		envJWTRefreshSecret: always,
		// JWT_SECRET_PREVIOUS: optional. PRD §8.2 documents it as
		// legitimately empty except during an active key rotation.
		envJWTSecretPrevious: nil,
		envDatabaseURL:       always,
		envProxyIP:           unlessDevelopment,
		envCorsOrigins:       always,
		envEncryptionKey:     always,
		envSMTPURL:           always,
		envMailFrom:          always,
		envAppEnv:            always,
		envPort:              always,
		envDatabaseURLRead:   nil, // optional (D-R)
		envSentryDSN:         nil, // optional
		envTurnstileSecret:   nil, // optional
	}

	switch cmd {
	case CommandServe:
		return serveVars
	case CommandMigrate:
		out := make(map[string]requiredFunc, len(serveVars)+3)
		for k, v := range serveVars {
			out[k] = v
		}
		out[envBootstrapDatabaseURL] = always
		out[envAppDBUser] = always
		out[envAppDBPassword] = always
		return out
	case CommandWorker:
		return map[string]requiredFunc{
			envDatabaseURL:       always,
			envAppEnv:            always,
			envDatabaseURLWorker: nil, // optional (D-R)
		}
	case CommandSeed:
		return map[string]requiredFunc{
			envDatabaseURL: always,
			envAppEnv:      always,
		}
	case CommandBootstrapSuperadmin:
		return map[string]requiredFunc{
			envDatabaseURL:   always,
			envEncryptionKey: always,
		}
	case CommandHealthReady:
		return map[string]requiredFunc{
			envPort: always,
		}
	case CommandHealthWorker:
		return map[string]requiredFunc{
			envDatabaseURL: always,
		}
	default:
		return nil
	}
}

// Load validates every environment variable cmd requires, builds the
// log-safe Config and the secrets.Holder, and -- once validation
// succeeds -- removes ENCRYPTION_KEY from the process environment
// (platform-bootstrap: ENCRYPTION_KEY Isolation).
func Load(_ context.Context, lookup LookupEnv, cmd Command) (Config, *secrets.Holder, error) {
	spec := requirementSet(cmd)
	if spec == nil {
		return Config{}, nil, fmt.Errorf("config: unknown command %q", cmd)
	}

	names := make([]string, 0, len(spec))
	for name := range spec {
		names = append(names, name)
	}
	sort.Strings(names)

	verr := &ValidationError{}
	values := make(map[string]string, len(names))

	for _, name := range names {
		val, present := lookup(name)
		values[name] = val

		required := spec[name]
		if required == nil || !required(lookup) {
			continue
		}
		if !present || val == "" {
			verr.Missing = append(verr.Missing, name)
		}
	}

	var encKey []byte
	if val := values[envEncryptionKey]; val != "" {
		decoded, err := base64.StdEncoding.DecodeString(val)
		if err != nil || len(decoded) != 32 {
			verr.Invalid = append(verr.Invalid, invalidVar{envEncryptionKey, "must decode to exactly 32 base64-encoded bytes"})
		} else {
			encKey = decoded
		}
	}

	if val := values[envAppEnv]; val != "" {
		switch AppEnv(val) {
		case AppEnvDevelopment, AppEnvStaging, AppEnvProduction:
		default:
			verr.Invalid = append(verr.Invalid, invalidVar{envAppEnv, "must be one of development, staging, production"})
		}
	}

	if len(verr.Missing) > 0 || len(verr.Invalid) > 0 {
		return Config{}, nil, verr
	}

	cfg := Config{
		AppEnv:      AppEnv(values[envAppEnv]),
		Domain:      values[envDomain],
		Port:        values[envPort],
		ProxyIP:     values[envProxyIP],
		MailFrom:    values[envMailFrom],
		AppDBUser:   values[envAppDBUser],
		CorsOrigins: splitCSV(values[envCorsOrigins]),
	}
	if host, err := dsnHost(values[envDatabaseURL]); err == nil {
		cfg.DatabaseHost = host
	}

	// D-R point 4: an unconfigured read/worker override falls back to
	// DATABASE_URL, at the config layer, so both handles work with no
	// new stack variable required.
	dbURLRead := values[envDatabaseURLRead]
	if dbURLRead == "" {
		dbURLRead = values[envDatabaseURL]
	}
	dbURLWorker := values[envDatabaseURLWorker]
	if dbURLWorker == "" {
		dbURLWorker = values[envDatabaseURL]
	}

	holder := secrets.NewHolder(secrets.Values{
		JWTSecret:            values[envJWTSecret],
		JWTSecretPrevious:    values[envJWTSecretPrevious],
		JWTRefreshSecret:     values[envJWTRefreshSecret],
		EncryptionKey:        encKey,
		DatabaseURL:          values[envDatabaseURL],
		DatabaseURLRead:      dbURLRead,
		DatabaseURLWorker:    dbURLWorker,
		BootstrapDatabaseURL: values[envBootstrapDatabaseURL],
		AppDBPassword:        values[envAppDBPassword],
		SMTPURL:              values[envSMTPURL],
		SentryDSN:            values[envSentryDSN],
		TurnstileSecret:      values[envTurnstileSecret],
	})

	// platform-bootstrap: ENCRYPTION_KEY Isolation -- once validation
	// succeeds, ENCRYPTION_KEY is reachable only through holder from
	// here on; Unsetenv is not a memory scrub, but it is what the PRD
	// asks for ("la app la lee al arrancar y la borra de process.env").
	unsetenvEncryptionKey()

	return cfg, holder, nil
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func dsnHost(dsn string) (string, error) {
	if dsn == "" {
		return "", fmt.Errorf("config: empty dsn")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("config: parse dsn: %w", err)
	}
	return u.Host, nil
}
