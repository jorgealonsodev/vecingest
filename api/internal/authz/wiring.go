package authz

import "github.com/jorgealonsodev/vecingest/internal/db"

// ResolverDBTX returns the database handle every scoped.* resolver
// reads through.
//
// It is the PRIMARY, never the read replica: an authorization decision
// MUST be read-your-writes. Every membership-producing write commits on
// the primary and hands the caller a session in the SAME response --
// AcceptInvitation inserts unit_members inside a d.DB.Write transaction
// and then issues a session, AddOfficeMember and CreateOffice insert
// office_members on d.DB.Write -- so resolving from a replica turns
// ordinary replication lag into ErrNoMembership, which
// scoped/register.go deliberately renders as a 404 the client cannot
// tell apart from a genuinely foreign resource (review lineage
// review-e72754dc7521b57a,
// R4-authz-resolves-from-read-replica-no-read-your-writes).
//
// The cost is deliberate and bounded: resolution is one indexed lookup
// per scoped request (design D-4), not a reporting read, and every
// handler that makes no authorization decision keeps reading the
// replica (db.New(d.DB.Read) in communities.go, me.go, offices.go, ...).
func ResolverDBTX(h db.Handles) db.DBTX { return h.Write }

// ConfigureFromHandles is the single Configure call site for a process
// that owns db.Handles (serve, and the HTTP integration harness), so the
// read-your-writes choice above is made in exactly one place instead of
// once per wiring site.
func ConfigureFromHandles(h db.Handles) { Configure(db.New(ResolverDBTX(h))) }
