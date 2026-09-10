// This file hosts another "startup row that builds the distroless image
// and probes it" (design.md Testing Strategy), same shape and same
// testing.Short() skip guard as docker_tzcheck_test.go, but aimed at the
// first real deployment's first two failures instead of the tzdata embed
// (docs/pendientes-despliegue.md item 10): the Go test suite runs the
// vecingest binary directly, so nothing ever exercised the Dockerfile,
// the ENTRYPOINT, or the argv the compose file's own `command:` produces
// once Docker concatenates it with that ENTRYPOINT.
package test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// TestDockerImage_EntrypointAndArchitectureRunOnHost builds the
// production vecingest image (api/Dockerfile's default, unnamed final
// stage -- the exact image docker-compose.yml's `api` and `worker`
// services run) and actually RUNS the resulting container with the `api`
// service's own `command:` arguments, read straight out of the real
// docker-compose.yml. Inspecting image metadata is deliberately never
// enough here and this test never does it: `docker image inspect` once
// reported the correct architecture while the binary copied inside the
// image was for the wrong one, because gcr.io/distroless/static-debian12
// is a multi-arch base whose manifest resolves to the *host's*
// architecture regardless of what got COPYed in during the build. Only
// actually executing the container surfaces that mismatch.
//
// Two of the first deployment's three failures are regressions this test
// catches:
//
//  1. `GOARCH` pinned to an architecture the host cannot run: the
//     container fails to start at all, with `exec format error` from the
//     container runtime (never from vecingest itself, which never gets a
//     chance to run).
//  2. `ENTRYPOINT ["/vecingest"]` plus a `command:` that repeats the
//     binary path (e.g. `["/vecingest", "serve", "--migrate"]`) makes
//     Docker concatenate them into argv, so main.go's dispatcher sees
//     "/vecingest" as the subcommand and rejects it with
//     `unknown subcommand "/vecingest"`.
//
// The third failure (docker-compose.yml's shallow `<<:` environment
// merge) is a compose-file-shape bug, not a container-execution bug, and
// is instead caught statically by lintcompose's shallow-merge-trap rule
// (api/cmd/lintcompose, docs/pendientes-despliegue.md item 11).
func TestDockerImage_EntrypointAndArchitectureRunOnHost(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: requires Docker and builds an image (docker build)")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("integration: docker CLI not available on PATH")
	}

	repoRoot, err := filepath.Abs(filepath.Join(".", "..", ".."))
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}

	apiCommand := composeAPICommand(t, filepath.Join(repoRoot, "docker-compose.yml"))

	tag := "vecingest-container-entrypoint-test:ci"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	buildCmd := exec.CommandContext(ctx, "docker", "build", //nolint:gosec // G204: fixed, hardcoded arguments only, no external input
		"-f", "api/Dockerfile",
		"-t", tag,
		".",
	)
	buildCmd.Dir = repoRoot
	buildOut, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker build (default target, production image) failed: %v\n%s", err, buildOut)
	}

	t.Cleanup(func() {
		rmCtx, rmCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer rmCancel()
		_ = exec.CommandContext(rmCtx, "docker", "rmi", "-f", tag).Run() //nolint:gosec // G204: fixed, hardcoded arguments only, best-effort cleanup
	})

	runArgs := append([]string{"run", "--rm", tag}, apiCommand...)
	runCmd := exec.CommandContext(ctx, "docker", runArgs...) //nolint:gosec // G204: `tag` is a fixed local constant, `apiCommand` comes from this repo's own docker-compose.yml read locally, never external/network input
	runOut, runErr := runCmd.CombinedOutput()
	got := string(runOut)

	// The container runs with NO environment at all, so it can never
	// reach a healthy `serve` loop -- that part is expected and not what
	// this test is checking. What matters is *which* error comes back:
	//
	//   - a clean (zero) exit would mean the container is somehow
	//     serving with no database configured, which is worse than any
	//     of the three failure modes below and must never happen.
	//   - "unknown subcommand" means main.go's dispatcher never received
	//     "serve" as argv[1]: the ENTRYPOINT/command concatenation bug.
	//   - "exec format error" (emitted by the container runtime, never by
	//     vecingest) means the binary inside the image cannot execute on
	//     this host's architecture at all.
	//   - "config validation failed" can ONLY be printed by vecingest's
	//     own config.Load (api/internal/config/config.go), which only
	//     runs after main.go correctly dispatched to runServe and
	//     runServe correctly parsed the compose file's own flags. An
	//     error here is therefore the PASSING signal: it proves the
	//     architecture is right and the entrypoint/command dispatch
	//     reached real vecingest code, not a container that never
	//     started or a CLI that never recognized its own subcommand.
	if runErr == nil {
		t.Fatalf("expected the container to exit non-zero with no config present, it exited 0 -- command was %v, output: %s", apiCommand, got)
	}
	if strings.Contains(got, "unknown subcommand") {
		t.Fatalf("ENTRYPOINT/command concatenation regression: dispatcher never reached %v, got: %s", apiCommand, got)
	}
	if strings.Contains(strings.ToLower(got), "exec format error") {
		t.Fatalf("architecture regression: the built image cannot execute on this host, got: %s", got)
	}
	if !strings.Contains(got, "config validation failed") {
		t.Fatalf("expected dispatch to reach config validation (proving both ENTRYPOINT/command dispatch and architecture work), got: %s", got)
	}
}

// composeAPICommand reads the `api` service's `command:` array straight
// out of the real docker-compose.yml, rather than hardcoding a copy of
// it, so this test exercises exactly what Portainer/`docker compose up`
// would run and fails the moment that array regresses (e.g. if
// `/vecingest` is reintroduced at its front) instead of silently testing
// a private, drifted expectation.
func composeAPICommand(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // G304: fixed repo-relative path, test-only
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	// Decoded generically (not into a typed `Command []string` struct
	// shared by every service): other services in this file (e.g. `db`)
	// declare `command:` as a plain shell string rather than an array,
	// which a shared struct shape would fail to unmarshal.
	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}

	services, ok := doc["services"].(map[string]interface{})
	if !ok {
		t.Fatalf("%s: no top-level `services` mapping", path)
	}
	api, ok := services["api"].(map[string]interface{})
	if !ok {
		t.Fatalf("%s: no `api` service found", path)
	}
	rawCommand, ok := api["command"].([]interface{})
	if !ok || len(rawCommand) == 0 {
		t.Fatalf("%s: `api` service has no array-form `command:` entries, got %#v", path, api["command"])
	}

	command := make([]string, 0, len(rawCommand))
	for _, item := range rawCommand {
		s, ok := item.(string)
		if !ok {
			t.Fatalf("%s: `api` service `command:` entry %#v is not a string", path, item)
		}
		command = append(command, s)
	}
	return command
}
