// Command lintcompose fails loudly when a service in a Compose file is
// missing a hardening key that D-K/§8.1 require it to carry. Compose has
// no admission controller: nothing else stops a service from silently
// losing `read_only`, `cap_drop`, `no-new-privileges`, a `tmpfs` mount or
// its non-root `user:` the next time the file is hand-edited. This tool
// is that missing admission controller, run structurally against the raw
// YAML (no `docker compose config`, no Docker daemon required), so it
// works in the fast CI lane too.
//
// Two services are deliberate, documented exceptions to full hardening,
// matching PRD_go.md §8.1's own reference copy verbatim (D-K):
//
//   - db: the official postgres image needs a writable filesystem for its
//     data directory, WAL and temp files, and drops privileges itself via
//     its own entrypoint; `read_only`/`cap_drop`/`tmpfs`/`user` would break
//     initdb and startup.
//   - clamav: an inert `profiles: [av]` stub at M0 (ClamAV is M2 scope,
//     PRD §7.7). It never runs unless a deployer explicitly opts into the
//     `av` profile, so it carries none of the hardening keys below.
//
// Usage:
//
//	go run ./cmd/lintcompose <path/to/docker-compose.yml> [<more files>...]
//	go run ./cmd/lintcompose ../docker-compose.yml   # run from api/, repo-root file
package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// expectation is the per-service hardening policy this tool enforces.
type expectation struct {
	fullHardening  bool // read_only, cap_drop:[ALL], no-new-privileges, tmpfs, non-root user
	requireMemory  bool // mem_limit present
	requireRestart bool // restart: unless-stopped
	requireNetwork bool // at least one entry under networks
	requirePorts   bool // at least one entry under ports -- published for the hand-configured nginx reverse proxy
	forbidPorts    bool // MUST NOT publish any port (db: never reachable from the host)
	exceptionNote  string
}

var policy = map[string]expectation{
	// site/web no longer need an explicit `networks:` entry: they are
	// reached by the hand-configured nginx reverse proxy through a
	// published host port (requirePorts), not through a shared docker
	// network. Compose's default network is enough for their own
	// outbound needs.
	"site": {fullHardening: true, requireMemory: true, requireRestart: true, requirePorts: true},
	"web":  {fullHardening: true, requireMemory: true, requireRestart: true, requirePorts: true},
	// api keeps `networks: [internal]` to reach db, and additionally
	// publishes a host port for the reverse proxy.
	"api": {fullHardening: true, requireMemory: true, requireRestart: true, requireNetwork: true, requirePorts: true},
	"worker": {
		fullHardening: true, requireMemory: true, requireRestart: true, requireNetwork: true,
	},
	"db": {
		fullHardening: false, requireMemory: true, requireRestart: true, requireNetwork: true, forbidPorts: true,
		exceptionNote: "db is exempt from full hardening: postgres needs a writable filesystem (D-K)",
	},
	"clamav": {
		fullHardening: false, requireMemory: true, requireRestart: true, requireNetwork: true,
		exceptionNote: "clamav is an inert M2 stub at M0, profiles:[av] (D-K)",
	},
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: lintcompose <path/to/docker-compose.yml> [<more files>...]")
		os.Exit(2)
	}

	var failures []string
	for _, path := range os.Args[1:] {
		failures = append(failures, lintFile(path)...)
	}

	if len(failures) > 0 {
		fmt.Fprintln(os.Stderr, "lintcompose: hardening gaps found:")
		for _, f := range failures {
			fmt.Fprintf(os.Stderr, "  - %s\n", f)
		}
		os.Exit(1)
	}

	fmt.Println("lintcompose: OK — every service carries its required hardening flags")
}

func lintFile(path string) []string {
	data, err := os.ReadFile(path) //nolint:gosec // G703: path traversal via taint analysis -- false positive. `path` is a literal CLI argument this operator/CI-invoked tool is started with (e.g. `go run ./cmd/lintcompose ../docker-compose.yml`), never untrusted network or request input; the same pattern gosec does not flag in cmd/openapi-gen's own file writes.
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", path, err)}
	}

	// Generic YAML tree-walk of an operator-supplied local file (see the
	// os.ReadFile note above), never used for a type-based security decision.
	var doc interface{} // nosemgrep: go.lang.security.deserialization.unsafe-deserialization-interface.go-unsafe-deserialization-interface
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return []string{fmt.Sprintf("%s: invalid YAML: %v", path, err)}
	}

	root, ok := doc.(map[string]interface{})
	if !ok {
		return []string{fmt.Sprintf("%s: top-level document is not a mapping", path)}
	}

	rawServices, ok := root["services"]
	if !ok {
		return []string{fmt.Sprintf("%s: no top-level `services` key", path)}
	}
	services, ok := rawServices.(map[string]interface{})
	if !ok {
		return []string{fmt.Sprintf("%s: `services` is not a mapping", path)}
	}

	var failures []string
	for name, rawSvc := range services {
		svc, ok := rawSvc.(map[string]interface{})
		if !ok {
			failures = append(failures, fmt.Sprintf("%s: service %q is not a mapping", path, name))
			continue
		}

		exp, known := policy[name]
		if !known {
			failures = append(failures, fmt.Sprintf(
				"%s: service %q has no lintcompose policy entry — add one to api/cmd/lintcompose/main.go (either full hardening or a documented, named exception)",
				path, name))
			continue
		}

		failures = append(failures, checkService(path, name, svc, exp)...)
	}

	failures = append(failures, checkShallowMergeTrap(path, data)...)

	return failures
}

func checkService(path, name string, svc map[string]interface{}, exp expectation) []string {
	var failures []string
	fail := func(format string, args ...interface{}) {
		failures = append(failures, fmt.Sprintf("%s: service %q: "+format, append([]interface{}{path, name}, args...)...))
	}

	if exp.fullHardening {
		if b, ok := svc["read_only"].(bool); !ok || !b {
			fail("missing `read_only: true`")
		}
		if !stringSliceContains(svc["cap_drop"], "ALL") {
			fail("missing `cap_drop: [ALL]`")
		}
		if !stringSliceContains(svc["security_opt"], "no-new-privileges:true") {
			fail("missing `security_opt: [no-new-privileges:true]`")
		}
		if isEmptySlice(svc["tmpfs"]) {
			fail("missing a non-empty `tmpfs` mount")
		}
		user, _ := svc["user"].(string)
		if user == "" || user == "0" || user == "0:0" || user == "root" {
			fail("missing a non-root `user:` (got %q)", user)
		}
	} else if exp.exceptionNote != "" {
		// Documented exception: intentionally not checked here. The
		// exception itself is asserted by the presence of this branch
		// plus the compose file's own inline comment.
		_ = exp.exceptionNote
	}

	if exp.requireMemory {
		if _, ok := svc["mem_limit"]; !ok {
			fail("missing `mem_limit`")
		}
	}

	if exp.requireRestart {
		if r, ok := svc["restart"].(string); !ok || r != "unless-stopped" {
			fail("missing `restart: unless-stopped`")
		}
	}

	if exp.requireNetwork {
		if isEmptyNetworks(svc["networks"]) {
			fail("missing a `networks` entry")
		}
	}

	if exp.requirePorts {
		if isEmptySlice(svc["ports"]) {
			fail("missing a `ports` entry -- the hand-configured nginx reverse proxy targets a published host port, not a shared docker network")
		}
	}

	if exp.forbidPorts {
		if !isEmptySlice(svc["ports"]) {
			fail("must NOT publish a `ports` entry -- this service must stay reachable only from the internal network")
		}
	}

	if name == "api" || name == "worker" {
		env, _ := svc["environment"].(map[string]interface{})
		if _, ok := env["GOMEMLIMIT"]; !ok {
			fail("missing `environment.GOMEMLIMIT` (D-K)")
		}
	}

	if name == "worker" {
		if _, ok := svc["stop_grace_period"]; !ok {
			fail("missing `stop_grace_period` (D-K: bounds graceful River shutdown)")
		}
	}

	return failures
}

func stringSliceContains(v interface{}, want string) bool {
	items, ok := v.([]interface{})
	if !ok {
		return false
	}
	for _, item := range items {
		if s, ok := item.(string); ok && s == want {
			return true
		}
	}
	return false
}

func isEmptySlice(v interface{}) bool {
	items, ok := v.([]interface{})
	return !ok || len(items) == 0
}

// isEmptyNetworks accepts both Compose `networks:` forms: the short list
// form (`[internal, proxy]`) and the long mapping form (per-network
// config keyed by network name).
func isEmptyNetworks(v interface{}) bool {
	switch n := v.(type) {
	case []interface{}:
		return len(n) == 0
	case map[string]interface{}:
		return len(n) == 0
	default:
		return true
	}
}

// checkShallowMergeTrap catches the regression this repo actually shipped:
// `<<:` is a YAML *merge key*, and the merge is SHALLOW. A service that
// merges an anchor (`<<: *app-image`, or `<<: [*app-image, *hardening]`)
// AND declares its own `environment:` key does not get the anchor's
// environment plus its own -- its own `environment:` REPLACES the merged
// one entirely, because a key a mapping declares locally always wins over
// the same key contributed by a merge. That is exactly how `api` and
// `worker` once booted with only `GOMEMLIMIT` set and nothing else.
//
// This has to be checked against the raw (pre-merge) YAML node tree, not
// against the `map[string]interface{}` decoded above: by the time `<<` is
// resolved into a plain map, the merge has already happened and the
// service's own `environment:` value is indistinguishable from one that
// was never overridden. gopkg.in/yaml.v3 resolves merge keys automatically
// when decoding into `interface{}`, but leaves them untouched (as `<<`
// mapping pairs with alias values) when decoding into `*yaml.Node`.
//
// The check is deliberately generic: it does not hardcode "x-app-image",
// "api" or "worker" anywhere. It asks, for every service, "does merging
// what this service merges bring in an `environment:` key, AND does this
// service also declare its own `environment:` key?" -- which is the shape
// of the trap itself, for any anchor and any service, present or future.
func checkShallowMergeTrap(path string, data []byte) []string {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		// The interface{} decode above already reported invalid YAML.
		return nil
	}
	if len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}
	servicesNode := mappingValue(root, "services")
	if servicesNode == nil || servicesNode.Kind != yaml.MappingNode {
		return nil
	}

	var failures []string
	for i := 0; i+1 < len(servicesNode.Content); i += 2 {
		name := servicesNode.Content[i].Value
		svcNode := resolveAlias(servicesNode.Content[i+1])
		if svcNode == nil || svcNode.Kind != yaml.MappingNode {
			continue
		}

		if !mappingOwnKeys(svcNode)["environment"] {
			continue // nothing declared locally -- no local key to clobber the merge
		}

		for _, src := range mergeSources(svcNode) {
			if resolvedKeySet(src, map[*yaml.Node]bool{})["environment"] {
				failures = append(failures, fmt.Sprintf(
					"%s: service %q: declares its own `environment:` key while also merging an anchor via `<<:` that defines `environment:` -- YAML's `<<:` merge is SHALLOW, so this service's local `environment:` REPLACES the merged one entirely instead of adding to it; move these variables into the merged anchor or drop the local `environment:` key",
					path, name))
				break
			}
		}
	}
	return failures
}

// mappingValue returns the value node for a scalar key directly declared
// in a mapping node, ignoring merge keys (`<<`).
func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// mappingOwnKeys returns the set of keys a mapping node declares directly
// (never following `<<` merges), excluding the merge key itself.
func mappingOwnKeys(node *yaml.Node) map[string]bool {
	keys := map[string]bool{}
	if node == nil || node.Kind != yaml.MappingNode {
		return keys
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i].Value
		if k == "<<" {
			continue
		}
		keys[k] = true
	}
	return keys
}

// mergeSources returns the anchor node(s) a mapping merges via `<<:`,
// supporting both `<<: *anchor` and `<<: [*anchor1, *anchor2]`. Returned
// nodes may themselves be alias nodes; callers resolve them.
func mergeSources(node *yaml.Node) []*yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	var sources []*yaml.Node
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value != "<<" {
			continue
		}
		v := node.Content[i+1]
		switch v.Kind {
		case yaml.SequenceNode:
			sources = append(sources, v.Content...)
		default:
			sources = append(sources, v)
		}
	}
	return sources
}

// resolveAlias follows an AliasNode to the node it points to, or returns
// the node unchanged if it is not an alias.
func resolveAlias(node *yaml.Node) *yaml.Node {
	if node != nil && node.Kind == yaml.AliasNode {
		return node.Alias
	}
	return node
}

// resolvedKeySet computes the effective set of top-level keys a mapping
// node would carry once every `<<:` merge it participates in (including
// merges nested inside anchors it merges) is applied, respecting the same
// shallow-merge precedence Compose/YAML use: a node's own keys always win
// over a key contributed only by something it merges. `visited` guards
// against alias cycles.
func resolvedKeySet(node *yaml.Node, visited map[*yaml.Node]bool) map[string]bool {
	node = resolveAlias(node)
	if node == nil || node.Kind != yaml.MappingNode {
		return map[string]bool{}
	}
	if visited[node] {
		return map[string]bool{}
	}
	visited[node] = true

	result := map[string]bool{}
	for _, src := range mergeSources(node) {
		for k := range resolvedKeySet(src, visited) {
			result[k] = true
		}
	}
	for k := range mappingOwnKeys(node) {
		result[k] = true
	}
	return result
}
