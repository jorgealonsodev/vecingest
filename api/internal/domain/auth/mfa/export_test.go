package mfa

// Base32SecretForTest exposes the unexported base32Secret so mfa_test can
// generate reference codes with the exact same encoding MatchStep uses.
func Base32SecretForTest(secret []byte) string {
	return base32Secret(secret)
}

// IsSerializationFailureForTest exposes the unexported
// isSerializationFailure classifier (D-P's pgerrcode 40001 branch).
func IsSerializationFailureForTest(err error) bool {
	return isSerializationFailure(err)
}
