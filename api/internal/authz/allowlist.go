// allowlist.go holds the D-3 reviewed public-route allowlist consulted
// by A3: every entry needs a stated reason, and no entry may sit under a
// tenant-scoped prefix (recovery from a false positive is an allowlist
// entry plus an issue, never disabling the assertion).
package authz

// Entry is one reviewed allowlist row: a route A3's chi-vs-OpenAPI diff
// legitimately finds (a public route registered via plain huma.Register,
// or a Hidden operation), together with why it is safe.
type Entry struct {
	Method string
	Path   string
	Reason string
}

// Allowlist is the full reviewed set A3 checks A3's difference set
// against.
type Allowlist []Entry

// Contains reports whether allow has an entry matching (method, path).
func (allow Allowlist) Contains(method, path string) bool {
	for _, e := range allow {
		if e.Method == method && e.Path == path {
			return true
		}
	}
	return false
}

// PublicOperations is the reviewed public-route allowlist A3 consults
// (design D-3). Seeded with M0's existing public routes: every one of
// these already passes A1 (not under a scoped prefix) and A3 (properly
// documented, not Hidden) on its own, but each is listed here anyway as
// a reviewed, per-entry-justified record of "this route is
// intentionally public" -- so a future change that makes one of them
// Hidden, or otherwise undocumented, is caught by A3 rather than
// silently passing.
//
// A route under /v1/communities/, /v1/units/ or /v1/offices/ MUST NEVER
// appear here (design D-3, enforced by
// TestPublicOperations_NoEntryUnderAScopedPrefix): recovery from a
// production false positive is a new entry here plus an issue, never
// disabling the assertion.
var PublicOperations = Allowlist{
	{Method: "POST", Path: "/v1/auth/login", Reason: "Public login endpoint; no membership to resolve yet."},
	{Method: "POST", Path: "/v1/auth/superadmin/login", Reason: "Public superadmin login endpoint; superadmin is not a tenant membership."},
	{Method: "POST", Path: "/v1/auth/forgot-password", Reason: "Public password-recovery entry point (public-form-protection)."},
	{Method: "POST", Path: "/v1/auth/reset-password", Reason: "Public password-reset completion; authenticated by a one-time token, not a session."},
	{Method: "POST", Path: "/v1/auth/refresh", Reason: "Refresh-token exchange; runs before an access token (and therefore any membership) exists."},
	{Method: "GET", Path: "/v1/auth/refresh/csrf", Reason: "CSRF-token issuance for the cookie transport; no membership to resolve."},
	{Method: "GET", Path: "/v1/health/live", Reason: "Liveness probe; must respond with no auth dependency at all."},
	{Method: "GET", Path: "/v1/health/ready", Reason: "Readiness probe; must respond with no auth dependency at all."},
}
