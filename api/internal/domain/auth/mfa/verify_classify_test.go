package mfa_test

import (
	"errors"
	"testing"

	"github.com/jorgealonsodev/vecingest/internal/domain/auth/mfa"
)

// fakeSQLStateErr implements the minimal interface{ SQLState() string }
// isSerializationFailure classifies against, without depending on a
// real pgconn.PgError.
type fakeSQLStateErr struct{ code string }

func (e fakeSQLStateErr) Error() string    { return "fake sql error: " + e.code }
func (e fakeSQLStateErr) SQLState() string { return e.code }

// D-P: "if internal/db ever raises the isolation level for any reason,
// mfa.VerifyTOTP MUST classify pgerrcode.SerializationFailure (40001)
// as the same outcome as zero rows".
func TestIsSerializationFailure_ClassifiesPgErrCode40001(t *testing.T) {
	if !mfa.IsSerializationFailureForTest(fakeSQLStateErr{code: "40001"}) {
		t.Fatalf("expected SQLSTATE 40001 to classify as a serialization failure")
	}
}

func TestIsSerializationFailure_RejectsOtherCodes(t *testing.T) {
	if mfa.IsSerializationFailureForTest(fakeSQLStateErr{code: "23505"}) {
		t.Fatalf("expected a unique-violation SQLSTATE to NOT classify as a serialization failure")
	}
}

func TestIsSerializationFailure_RejectsPlainErrors(t *testing.T) {
	if mfa.IsSerializationFailureForTest(errors.New("plain error")) {
		t.Fatalf("expected a plain error with no SQLState() to NOT classify as a serialization failure")
	}
}
