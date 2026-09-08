package hibp

import "github.com/jorgealonsodev/vecingest/internal/domain/auth/password"

// Compile-time proof that Client implements the domain-owned
// password.HIBPChecker port. hibp is the platform adapter; the port
// itself is defined in internal/domain/auth/password (D-F, D-G).
var _ password.HIBPChecker = (*Client)(nil)
