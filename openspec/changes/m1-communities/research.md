# Research — m1-communities

Lane: **how do you make an authorization check structurally impossible to omit
in a Go HTTP handler?**

Collected 2026-09-15 for the decision `PRD_go.md` §7.7 leaves open, which the
exploration identified as M1's architectural question. Evidence is external
(documentation and primary sources); the repository-side facts were verified
separately by the orchestrator and are marked as such.

> **Why this file exists.** The `sdd-research` agent has only `WebFetch`,
> `WebSearch` and `codegraph_explore` — no write tool and no Engram tools — so
> it could persist nothing, to either half of the `hybrid` store. Its findings
> survived only in its handoff message and were transcribed here. The same gap
> hit `sdd-explore`, which has no write tool either. Anything a research phase
> produces has to be written down by whoever receives it.

## Sources

| | Class | Source |
| --- | --- | --- |
| S1 | documentation | [Middleware — Huma](https://huma.rocks/features/middleware/) |
| S2 | documentation | [OAuth 2.0 & JWT — Huma](https://huma.rocks/how-to/oauth2-jwt/) |
| S3 | documentation | [Request Resolvers — Huma](https://huma.rocks/features/request-resolvers/) |
| S4 | primary source | [`huma.go`, danielgtaylor/huma](https://raw.githubusercontent.com/danielgtaylor/huma/main/huma.go) |
| S5 | documentation | [`chi` router-walk example](https://github.com/go-chi/chi/blob/master/_examples/router-walk/main.go) |
| S6 | named article, real code | [How Render Enforces Access Controls with Go Generics](https://render.com/blog/how-render-enforces-access-controls-with-go-generics) |
| S7 | documentation | [`golang.org/x/tools/go/analysis`](https://pkg.go.dev/golang.org/x/tools/go/analysis) |
| S8 | primary source | [`humachi.go`, danielgtaylor/huma](https://github.com/danielgtaylor/huma/blob/main/adapters/humachi/humachi.go) |

## Findings

**C1 — huma owns the handler call site.** [S4, and verified locally against the
pinned version] `func Register[I, O any](api API, op Operation, handler
func(context.Context, *I) (*O, error))`. The orchestrator confirmed this byte
for byte in the module cache at **huma v2.39.1**, the version pinned in
`api/go.mod` — not on `main`, where the research originally read it. huma always
passes a plain `context.Context`, so **no handler signature can be made to
require an authorized-identity type**. §7.7's promise that "a handler does not
compile without it" is unachievable at the handler layer with this framework.

**C2 — middleware attachment is opt-in.** [S1] `huma.WithValue` propagates into
the standard context; `api.UseMiddleware` is global and `Operation.Middlewares`
per-operation. Neither is enforced by the router.

**C3 — huma's own authorization middleware fails open.** [S2] Its reference
implementation reads `ctx.Operation().Security` and, when that is empty, calls
`next(ctx)` with no check at all. An operation that simply omits `Security` is
unauthenticated by design. This reproduces exactly the risk §7.7 exists to close,
which is why it cannot be the mechanism.

**C4 — resolvers move the problem, they do not solve it.** [S3]
`Resolve(ctx huma.Context) []error` runs automatically before the handler, so it
could carry authorization. But nothing stops a new input struct from not
implementing the interface — the same "someone has to remember" gap, relocated.

**C5 — compile-time enforcement is real, one layer down.** [S6] Render's
production pattern: `type AuthorizedProject[T ProjectPermission] struct { project
*Project; permission T }`, constructible only through `AuthorizeProject()`, with
the permission as a generic marker so a mismatch is a compile error. Privileged
methods take that type as a parameter: `func (p *Project) Delete(auth
AuthorizedProject[ProjectDeletePermission])`.

**C6 — but its guarantee is scoped to the methods that declare it.** [inference
from S6] Applied here it would enforce "this repository method cannot be called
without a validly-constructed membership token". It does not, by itself,
guarantee every handler checks membership — only that every *gated data path*
does. No prior art was found applying it at the HTTP registration boundary.

**C7 — a `chi.Walk` audit reads the wrong layer.** [S5 + S8] humachi registers
each huma operation as a single opaque closure
(`router.MethodFunc(op.Method, op.Path, func(w, r) { handler(NewContext(...)) })`),
so huma's `Security` and per-operation middleware live *inside* that closure,
invisible to chi's middleware chain. A chi-walking test can assert only
chi-level middleware. It moves the drift hole rather than closing it.

**C8 — `api.OpenAPI()` is huma's own single source of truth.** [S4 + huma docs]
Every registered operation is fed into the OpenAPI model, `Security` included.
A check enumerating it has no second, hand-maintained list to drift against —
structurally different from a route list someone must keep in sync. Still
test-time or boot-time, not compile-time. *(The exact iteration surface —
`Operations()` versus walking `Paths` — was not confirmed and needs checking
against v2.39.1 before implementation.)*

**C9 — a custom linter needs SSA to be trustworthy.** [S7] `go/analysis` gives
per-package AST and type information but no cross-package call graph. Answering
"does this handler ever reach `RequireMembership`, through helpers or
interfaces" requires building call-graph analysis on top (`buildssa`). A naive
AST pass misses indirection — which is precisely the reliability risk.

## The ladder

The ordering is the answer, not the list.

| Technique | Guarantee | Closes the drift hole? |
| --- | --- | --- |
| Handler-signature typing | **foreclosed** (C1) | — |
| Opaque generic token on service methods | compile-time, below the handler (C5, C6) | only if every data path is gated |
| Registration-time wrapper requiring scope | registration-time | moves it to "did everyone use the wrapper" |
| `api.OpenAPI()` enumeration assertion | boot- or test-time, right layer (C8) | yes, structurally |
| `chi.Walk` audit | test-time, wrong layer (C7) | no |
| Custom `go/analysis` pass | lint-time (C9) | only with SSA work |

## Gaps and cautions

- The "registration-time wrapper" option has no found prior art for huma; its
  shape and cost are inference, not attested practice.
- huma v2 is under active development. Any field name or behaviour relied on
  here must be re-checked against the pinned version before the design commits
  to it.

## Resolved after the fact

The two open questions above about the introspection surface were answered by
reading v2.39.1 in the module cache during `sdd-design`. They are recorded here
so nobody re-investigates them, and because one of the answers **invalidates a
claim this document helped put into the proposal**.

- **Enumerating operations**: there is no `Operations()` method on `*OpenAPI`.
  The only mutator is `AddOperation` (`openapi.go:1509`); enumeration means
  walking `Paths map[string]*PathItem` (`openapi.go:1463`) across the eight
  per-method `*Operation` fields, as huma's own duplicate-id check does at
  `openapi.go:1519-1528`.
- **`Operation.Middlewares`**: reachable, and still useless for an assertion.
  It is exported at `openapi.go:901`, but `type Middlewares []func(ctx Context,
  next func(Context))` (`chain.go:5`) holds func values, which Go cannot
  compare, and group middleware never lands there — `Register` composes it from
  `api.Middlewares()` at `huma.go:881`.
- **C8 is too strong, and the correction matters.** `api.OpenAPI()` is huma's
  single source of truth for *documented* operations, not for *registered
  routes*. `Hidden` appears exactly once in `huma.go`, at line 816, guarding
  only `AddOperation`; routing at `huma.go:881` and `humachi.go:167-170` is
  unconditional. So `Hidden: true` yields a served route absent from `Paths`,
  and a boot assertion reading only the OpenAPI model is defeated by one struct
  field. The design closes this by asserting over two surfaces — see D-2's A3,
  which diffs the `chi.Walk` route set against the documented set.
- **C7 still stands.** A3's use of `chi.Walk` is not a reversal: C7 rejected it
  for inspecting `Security` and per-operation middleware, which humachi buries
  inside an opaque closure. Whether a route *exists* is a different question,
  and the one `chi.Walk` answers authoritatively.
