package token

// KIDForTest exposes kidFor to this package's external test package,
// mirroring internal/domain/auth/mfa's export_test.go. It exists so a
// test can hand-mint a token in a SHAPE this package no longer issues
// (one with no mfa claim, as every token already in circulation on the
// deployed stack is) and still have VerifyAccess select the right
// secret. Duplicating the kid derivation in the test would prove
// nothing about the real one.
func KIDForTest(secret []byte) string { return kidFor(secret) }
