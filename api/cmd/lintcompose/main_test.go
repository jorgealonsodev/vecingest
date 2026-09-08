package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLintFile_RealComposeFilePasses is a regression test: the actual
// repo-root docker-compose.yml this repository ships MUST always pass
// its own lint. If this test fails, either the compose file drifted or
// the policy table in main.go needs a deliberate, documented update.
func TestLintFile_RealComposeFilePasses(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docker-compose.yml")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("docker-compose.yml not found at %s: %v", path, err)
	}

	failures := lintFile(path)
	if len(failures) != 0 {
		t.Fatalf("expected the real compose file to pass, got failures: %v", failures)
	}
}

// TestLintFile_MissingHardeningKeyFails proves the lint is not a no-op:
// a service known to the policy table but missing a required hardening
// key must fail, by name, with that key named in the message.
func TestLintFile_MissingHardeningKeyFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	// `api` requires full hardening; this fixture omits cap_drop and
	// security_opt entirely to prove the check actually fires.
	contents := `
services:
  api:
    image: example/api:latest
    read_only: true
    tmpfs: [/tmp]
    user: "65532:65532"
    mem_limit: 256m
    restart: unless-stopped
    networks: [internal]
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	failures := lintFile(path)
	if len(failures) == 0 {
		t.Fatal("expected at least one failure for a service missing cap_drop/security_opt")
	}

	joined := strings.Join(failures, "\n")
	if !strings.Contains(joined, "cap_drop") {
		t.Errorf("expected a cap_drop failure, got: %s", joined)
	}
	if !strings.Contains(joined, "security_opt") {
		t.Errorf("expected a security_opt failure, got: %s", joined)
	}
}

// TestLintFile_UnknownServiceRequiresPolicyEntry proves a service with no
// policy entry fails loudly instead of silently passing unchecked.
func TestLintFile_UnknownServiceRequiresPolicyEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	contents := `
services:
  mystery:
    image: example/mystery:latest
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	failures := lintFile(path)
	if len(failures) != 1 {
		t.Fatalf("expected exactly one failure for an unknown service, got: %v", failures)
	}
	if !strings.Contains(failures[0], "no lintcompose policy entry") {
		t.Errorf("expected an unknown-service policy failure, got: %s", failures[0])
	}
}

// TestLintFile_DBExceptionDoesNotRequireFullHardening proves the
// documented db exception (D-K) does not itself trigger a hardening
// failure, while still requiring mem_limit/restart/networks.
func TestLintFile_DBExceptionDoesNotRequireFullHardening(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")
	contents := `
services:
  db:
    image: postgres:17-alpine
    security_opt: [no-new-privileges:true]
    mem_limit: 768m
    restart: unless-stopped
    networks: [internal]
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	failures := lintFile(path)
	if len(failures) != 0 {
		t.Fatalf("expected db's documented exception to pass without full hardening, got: %v", failures)
	}
}
