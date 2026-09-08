package config

import "os"

// unsetenvEncryptionKey removes ENCRYPTION_KEY from the real process
// environment. It is a named, single-purpose wrapper (rather than an
// inline os.Unsetenv call) so ENCRYPTION_KEY Isolation stays a single,
// grep-able call site as more config surface is added in later phases.
func unsetenvEncryptionKey() {
	_ = os.Unsetenv(envEncryptionKey)
}
