// assert.go implements the D-2 rung-2 fail-closed boot assertion,
// AssertScopedRegistration, over three independently testable checks:
//
//   - A1: every scoped-prefixed operation documented in the OpenAPI
//     model carries the D-2 registration marker.
//   - A2: no marked operation exists without a registered resolver (and,
//     for Community/Office, a non-empty allowed-role set).
//   - A3: the route set chi actually serves, minus the OpenAPI-documented
//     set, is a subset of the reviewed public-route allowlist -- the
//     check that closes V7's Hidden-operation blind spot, since a
//     Hidden: true operation is routed unconditionally but never added
//     to Paths (huma.go:813-818, adapters/humachi/humachi.go:167-170).
package authz

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
)

// scopedPathPrefixes are the literal path prefixes A1 treats as
// tenant-scoped (design D-2). Every operation under one of these MUST
// carry the registration marker.
var scopedPathPrefixes = []string{"/v1/communities", "/v1/units", "/v1/offices"}

// invitationsPrefix is handled separately from scopedPathPrefixes: only
// "/v1/invitations/{invitationId}"-shaped routes are tenant-scoped.
// "/v1/invitations/preview" is a public, unauthenticated route with no
// tenant resource in the path and must NOT be forced to carry a marker.
const invitationsPrefix = "/v1/invitations/"

// isScopedPath reports whether path is a tenant-scoped route under A1's
// rule.
func isScopedPath(path string) bool {
	for _, p := range scopedPathPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	if rest, ok := strings.CutPrefix(path, invitationsPrefix); ok {
		return strings.HasPrefix(rest, "{")
	}
	return false
}

// resolverRegisteredKinds is the set of scope kinds this package can
// actually resolve (A2). A marker declaring any other Kind blocks boot.
var resolverRegisteredKinds = map[Kind]bool{
	KindCommunity: true,
	KindOffice:    true,
	KindSelf:      true,
}

// kindRequiresRoles reports whether A2 must also check a non-empty role
// set for this Kind. Self carries no role restriction by design (it
// returns the caller's own membership set; the handler decides what to
// expose), so an empty role set is legitimate there.
func kindRequiresRoles(k Kind) bool {
	return k == KindCommunity || k == KindOffice
}

// methodFields walks a PathItem's eight per-method *Operation fields —
// there is no Operations() method on *OpenAPI (V2), so every enumerator
// in this file copies huma's own duplicate-operation-ID check
// (openapi.go:1519-1528) rather than inventing a different walk.
func methodFields(item *huma.PathItem) map[string]*huma.Operation {
	return map[string]*huma.Operation{
		http.MethodGet:     item.Get,
		http.MethodPut:     item.Put,
		http.MethodPost:    item.Post,
		http.MethodDelete:  item.Delete,
		http.MethodHead:    item.Head,
		http.MethodOptions: item.Options,
		http.MethodPatch:   item.Patch,
		http.MethodTrace:   item.Trace,
	}
}

// AssertScopedRegistration runs A1, A2 and A3 in order and returns the
// first failure, naming the offending operation or route. serve calls
// this once after api.New(...) and before ListenAndServe; a non-nil
// error MUST abort startup (no panic outside main, per
// rules.apply.guidelines).
func AssertScopedRegistration(oapi *huma.OpenAPI, r chi.Router, allow Allowlist) error {
	if err := assertA1AndA2(oapi); err != nil {
		return err
	}
	return assertA3(oapi, r, allow)
}

// assertA1AndA2 walks Paths once, running A1 (marker present on every
// scoped-prefixed operation) and A2 (every marker has a registered
// resolver and, where required, a non-empty role set) together, since
// both iterate the same Paths × eight-method-fields shape (V3).
func assertA1AndA2(oapi *huma.OpenAPI) error {
	for path, item := range oapi.Paths {
		for method, op := range methodFields(item) {
			if op == nil {
				continue
			}

			marker, hasMarker := op.Metadata[MetadataKey].(Marker)

			if !hasMarker {
				if isScopedPath(path) {
					return fmt.Errorf("authz: A1 failed: operation %q (%s %s) is under a scoped prefix but carries no registration marker -- register it through scoped.Community/scoped.Office/scoped.Self", op.OperationID, method, path)
				}
				continue
			}

			if !resolverRegisteredKinds[marker.Kind] {
				return fmt.Errorf("authz: A2 failed: operation %q (%s %s) declares scope kind %v, which has no registered resolver", op.OperationID, method, path, marker.Kind)
			}
			if kindRequiresRoles(marker.Kind) && len(marker.Roles) == 0 {
				return fmt.Errorf("authz: A2 failed: operation %q (%s %s) declares scope kind %v with an empty role set", op.OperationID, method, path, marker.Kind)
			}
		}
	}
	return nil
}

// frameworkRoutes are huma's own served meta-endpoints -- its OpenAPI
// spec (JSON/YAML, 3.0-downgraded and not), its docs UI, and its
// JSON-schema route. huma.DefaultConfig registers every one of these via
// a.Handle directly (api.go New()), the exact same AddOperation bypass
// V7 documents for a Hidden operation: routed by chi, never added to
// Paths. They are identical for every deployment and carry no tenant
// data, so A3 treats them as framework infrastructure rather than
// requiring every reviewed application Allowlist to remember them.
var frameworkRoutes = map[string]bool{
	"GET /openapi.json":     true,
	"GET /openapi-3.0.json": true,
	"GET /openapi.yaml":     true,
	"GET /openapi-3.0.yaml": true,
	"GET /docs":             true,
	"GET /schemas/{schema}": true,
}

// assertA3 diffs the route set chi actually serves against the set
// api.OpenAPI() documents; anything served-but-undocumented must be in
// the reviewed allowlist. This is the check that closes V7: a
// Hidden: true operation routes unconditionally (humachi.go:167-170) but
// is never added to Paths (huma.go:813-818), so it is invisible to A1
// and only ever surfaces here.
func assertA3(oapi *huma.OpenAPI, r chi.Router, allow Allowlist) error {
	documented := documentedRouteSet(oapi)

	var undocumented []string
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = strings.TrimSuffix(route, "/*")
		key := routeKey(method, route)
		if documented[key] || frameworkRoutes[key] {
			return nil
		}
		if allow.Contains(method, route) {
			return nil
		}
		undocumented = append(undocumented, fmt.Sprintf("%s %s", method, route))
		return nil
	})
	if err != nil {
		return fmt.Errorf("authz: A3 failed: chi.Walk error: %w", err)
	}
	if len(undocumented) > 0 {
		return fmt.Errorf("authz: A3 failed: route(s) served by chi but neither documented in the OpenAPI model nor allowlisted: %s", strings.Join(undocumented, ", "))
	}
	return nil
}

// documentedRouteSet builds the (method, path) set api.OpenAPI()
// documents, keyed identically to chi.Walk's own (method, route) shape.
func documentedRouteSet(oapi *huma.OpenAPI) map[string]bool {
	set := make(map[string]bool)
	for path, item := range oapi.Paths {
		for method, op := range methodFields(item) {
			if op != nil {
				set[routeKey(method, path)] = true
			}
		}
	}
	return set
}

func routeKey(method, path string) string { return method + " " + path }

// EagerCheck is an additive, eager registration-time trigger (design
// D-2 / V4): wired as an OnAddOperation hook on the huma Config, it
// panics immediately when a scoped-prefixed operation is added with no
// authz marker, giving the earliest possible error site during route
// registration. It is deliberately NOT the sole check -- V4's own doc
// comment names the escape ("You may bypass this by directly writing to
// the Paths map instead"), and it can never fire for a Hidden operation
// (V7), since Hidden operations skip AddOperation entirely.
// AssertScopedRegistration (A1/A2/A3) remains the authoritative,
// non-panic, fail-closed boot-time gate.
func EagerCheck(_ *huma.OpenAPI, op *huma.Operation) {
	if op == nil {
		return
	}
	if _, hasMarker := op.Metadata[MetadataKey].(Marker); hasMarker {
		return
	}
	if isScopedPath(op.Path) {
		panic(fmt.Sprintf("authz: operation %q (%s %s) is under a scoped prefix but was registered with no authz marker -- register it through scoped.Community/scoped.Office/scoped.Self", op.OperationID, op.Method, op.Path))
	}
}
