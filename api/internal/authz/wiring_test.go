package authz

import (
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/db"
)

// R4-authz-resolves-from-read-replica-no-read-your-writes (review
// lineage review-e72754dc7521b57a). Every membership-producing write
// commits on the PRIMARY and the caller is handed a session in the SAME
// response: AcceptInvitation inserts unit_members inside a d.DB.Write
// transaction and then issues a session, AddOfficeMember and
// CreateOffice insert office_members on d.DB.Write. Resolving the
// caller's membership from the read replica therefore turns ordinary
// replication lag into ErrNoMembership on that caller's very next
// request -- and scoped/register.go renders ErrNoMembership as a 404,
// so the client cannot tell lag from a genuinely foreign resource and
// has no signal to retry. An authorization decision MUST be
// read-your-writes.
func TestResolverDBTX_IsThePrimaryNeverTheReadReplica(t *testing.T) {
	var handles db.Handles

	got := ResolverDBTX(handles)
	if got == db.DBTX(handles.Read) {
		t.Fatalf("authz resolvers are wired to the READ REPLICA: a membership just granted on the primary is invisible to the very next authorization decision")
	}
	if got != db.DBTX(handles.Write) {
		t.Fatalf("authz resolvers must read through the PRIMARY handle (authorization is read-your-writes), got %T", got)
	}
}

// ConfigureFromHandles must actually install a Querier -- a wiring
// function that silently left queries nil would leave every resolver
// returning errNotConfigured while the assertion above still passed.
func TestConfigureFromHandles_InstallsTheQuerier(t *testing.T) {
	t.Cleanup(func() { queries = nil })
	queries = nil

	ConfigureFromHandles(db.Handles{})
	if queries == nil {
		t.Fatalf("ConfigureFromHandles left the resolver Querier unconfigured")
	}
}
