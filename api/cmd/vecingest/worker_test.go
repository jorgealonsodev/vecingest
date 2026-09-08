package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jorgealonsodev/vecingest/internal/platform/migrate"
)

const workerTestAppDBPassword = "test-worker-subcommand-password" //nolint:gosec // G101: fake fixture password, Testcontainers-only

// buildVecingestBinary compiles the real cmd/vecingest binary into a
// temp dir once per test, for the one M0 scenario that genuinely needs a
// real separate OS process and real signal delivery: worker's graceful
// shutdown on SIGTERM (design D-S; platform-bootstrap: CLI Subcommands).
func buildVecingestBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	binPath := filepath.Join(dir, "vecingest")
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build vecingest binary: %v\n%s", err, out)
	}
	return binPath
}

// startMigratedPostgres starts a real Postgres 17 container, applies the
// bootstrap and schema migration sets, and returns the app_rw DSN plus
// the container itself (so this test can terminate it mid-test to
// exercise "health --worker exits 1 after the DB is stopped").
func startMigratedPostgres(t *testing.T) (appRWDSN string, container *postgres.PostgresContainer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	container, err := postgres.Run(ctx,
		"postgres:17-alpine",
		postgres.WithDatabase("vecingest"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	superuserDSN, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	superuserDB, err := sql.Open("pgx", superuserDSN)
	if err != nil {
		t.Fatalf("failed to open superuser db: %v", err)
	}
	defer func() { _ = superuserDB.Close() }()

	t.Setenv("APP_DB_USER", "app_rw")
	t.Setenv("APP_DB_PASSWORD", workerTestAppDBPassword)
	if err := migrate.RunBootstrap(ctx, superuserDB); err != nil {
		t.Fatalf("RunBootstrap failed: %v", err)
	}
	if err := migrate.RunSchema(ctx, superuserDB); err != nil {
		t.Fatalf("RunSchema failed: %v", err)
	}

	const scheme = "postgres://"
	rest := strings.TrimPrefix(superuserDSN, scheme)
	at := strings.IndexByte(rest, '@')
	appRWDSN = scheme + "app_rw:" + workerTestAppDBPassword + "@" + rest[at+1:]
	return appRWDSN, container
}

// platform-bootstrap: CLI Subcommands / D-S -- `worker` starts with zero
// registered workers, acquires River leadership, and shuts down cleanly
// on SIGTERM inside the compose stop_grace_period (30s); `health
// --worker` exits 0 while it runs and 1 after the DB is stopped. This is
// the one M0 subcommand scenario that needs a real built binary and real
// OS signal delivery (unlike bootstrap-superadmin/seed/health, which are
// exercised via direct in-process function calls).
func TestWorkerSubcommand_LeadershipAndGracefulShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Testcontainers-backed test in -short mode")
	}

	binPath := buildVecingestBinary(t)
	dsn, container := startMigratedPostgres(t)

	encKey := base64.StdEncoding.EncodeToString(make([]byte, 32))
	env := append(os.Environ(),
		"DATABASE_URL="+dsn,
		"APP_ENV=development",
		"ENCRYPTION_KEY="+encKey,
	)

	workerCmd := exec.Command(binPath, "worker")
	workerCmd.Env = env
	stdoutPipe, err := workerCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := workerCmd.Start(); err != nil {
		t.Fatalf("start worker: %v", err)
	}

	startedCh := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "worker: started") {
				close(startedCh)
			}
		}
	}()

	select {
	case <-startedCh:
	case <-time.After(20 * time.Second):
		_ = workerCmd.Process.Kill()
		t.Fatalf("worker did not report started within timeout")
	}

	// health --worker while the worker is running: exit 0.
	healthCmd := exec.Command(binPath, "health", "--worker")
	healthCmd.Env = env
	if out, err := healthCmd.CombinedOutput(); err != nil {
		t.Fatalf("expected health --worker to exit 0 while worker is running, got err=%v output=%s", err, out)
	}

	// SIGTERM must produce a clean, timely shutdown.
	if err := workerCmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}

	waitErrCh := make(chan error, 1)
	go func() { waitErrCh <- workerCmd.Wait() }()

	select {
	case err := <-waitErrCh:
		if err != nil {
			t.Fatalf("expected worker to exit 0 after SIGTERM, got: %v", err)
		}
	case <-time.After(30 * time.Second):
		_ = workerCmd.Process.Kill()
		t.Fatalf("worker did not shut down within the compose stop_grace_period (30s)")
	}

	// health --worker after the database itself is stopped: exit 1.
	if err := container.Stop(context.Background(), nil); err != nil {
		t.Fatalf("stop postgres container: %v", err)
	}

	healthAfterStopCmd := exec.Command(binPath, "health", "--worker")
	healthAfterStopCmd.Env = env
	if out, err := healthAfterStopCmd.CombinedOutput(); err == nil {
		t.Fatalf("expected health --worker to exit non-zero after the database is stopped, got output=%s", out)
	}
}
