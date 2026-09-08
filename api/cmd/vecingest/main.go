// Command vecingest is the single static binary exposing every M0
// subcommand: serve, worker, migrate, seed, bootstrap-superadmin, and
// health (platform-bootstrap: CLI Subcommands).
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata" // embeds the IANA tz database so the zone database travels inside the binary regardless of base image

	"github.com/jorgealonsodev/vecingest/internal/config"
)

// version is stamped at build time via
// `-ldflags "-X main.version=<value>"` (D-K, api/Dockerfile). It defaults
// to "dev" for local `go build`/`go run` invocations that pass no ldflags.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.LookupEnv))
}

// run dispatches to exactly one subcommand's own flow and never any
// other's (platform-bootstrap: CLI Subcommands, "Subcommand dispatch"
// scenario). It returns a process exit code rather than calling
// os.Exit itself, so it is directly testable in-process.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer, lookup config.LookupEnv) int {
	if len(args) < 1 {
		_, _ = fmt.Fprintln(stderr, "usage: vecingest <serve|worker|migrate|seed|bootstrap-superadmin|health> [flags]")
		return 2
	}

	sub, rest := args[0], args[1:]

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var err error
	switch sub {
	case "serve":
		err = runServe(ctx, rest, stdout, lookup)
	case "worker":
		err = runWorker(ctx, rest, stdout, lookup)
	case "migrate":
		err = runMigrate(ctx, rest, stdout, lookup)
	case "seed":
		err = runSeed(ctx, rest, stdout, lookup)
	case "bootstrap-superadmin":
		err = runBootstrapSuperadmin(ctx, rest, stdin, stdout, lookup)
	case "health":
		err = runHealth(ctx, rest, lookup)
	default:
		_, _ = fmt.Fprintf(stderr, "vecingest: unknown subcommand %q\n", sub)
		return 2
	}

	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
