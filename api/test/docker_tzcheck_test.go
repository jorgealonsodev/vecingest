// This file hosts the "startup row that builds the distroless image and
// probes it" (design.md Testing Strategy) — one of the seven slow rows
// requiring a testing.Short() skip guard. It is a binary-level regression
// test for the time/tzdata embed (task 8.4/8.5), never a workaround for a
// missing package: gcr.io/distroless/static already ships tzdata, but
// cmd/vecingest/main.go embeds it anyway per PRD §9 so the zone database
// travels inside the binary regardless of base image.
package test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDockerImage_TZDataEmbedResolvesEuropeMadrid builds the api/Dockerfile
// "tzcheck" stage (a build-only probe binary, never the production image's
// default target — see api/Dockerfile and api/cmd/tzcheck) and runs the
// resulting container, asserting that time.LoadLocation("Europe/Madrid")
// succeeds from inside the exact same gcr.io/distroless/static-debian12
// base image used for the real vecingest binary.
func TestDockerImage_TZDataEmbedResolvesEuropeMadrid(t *testing.T) {
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

	tag := "vecingest-tzcheck-test:ci"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	buildCmd := exec.CommandContext(ctx, "docker", "build", //nolint:gosec // G204: fixed, hardcoded arguments only, no external input
		"--target", "tzcheck",
		"-f", "api/Dockerfile",
		"-t", tag,
		".",
	)
	buildCmd.Dir = repoRoot
	buildOut, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker build --target tzcheck failed: %v\n%s", err, buildOut)
	}

	t.Cleanup(func() {
		rmCtx, rmCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer rmCancel()
		_ = exec.CommandContext(rmCtx, "docker", "rmi", "-f", tag).Run() //nolint:gosec // G204: fixed, hardcoded arguments only, best-effort cleanup
	})

	runCmd := exec.CommandContext(ctx, "docker", "run", "--rm", tag) //nolint:gosec // G204: fixed, hardcoded arguments only, no external input
	runOut, err := runCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker run %s failed: %v\n%s", tag, err, runOut)
	}

	got := string(runOut)
	if !strings.Contains(got, "tzcheck: ok zone=Europe/Madrid") {
		t.Fatalf("expected tzcheck to report a resolved Europe/Madrid zone, got: %q", got)
	}
}
