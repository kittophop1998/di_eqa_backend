// Package port contains application-owned ports for capabilities supplied by
// the outside world. Implementations belong in adapter/.
package port

import (
	"errors"
	"time"
)

// ErrTokenExpired is returned by TokenVerifier when the token is valid but expired.
var ErrTokenExpired = errors.New("token expired")

// TokenClaims is the authenticated identity exchanged between the application
// and its token adapter. It carries only the subject: role, status and hospital
// are always re-read from the database (BR-02).
type TokenClaims struct {
	UserID string
}

// TokenIssuer creates a signed token for an authenticated identity.
type TokenIssuer interface {
	Issue(TokenClaims) (token string, expiresAt time.Time, err error)
}

// PasswordHasher protects and verifies passwords. The application never
// depends on a particular algorithm such as bcrypt or argon2.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}
