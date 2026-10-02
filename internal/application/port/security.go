// Package port contains application-owned ports for capabilities supplied by
// the outside world. Implementations belong in adapter/.
package port

// TokenClaims is the authenticated identity exchanged between the application
// and its token adapter. It intentionally has no JWT or HTTP dependency.
type TokenClaims struct {
	UserID     string
	HospitalID string
	Role       string
}

// TokenIssuer creates a signed token for an authenticated identity.
type TokenIssuer interface {
	Issue(TokenClaims) (string, error)
}

// PasswordHasher protects and verifies passwords. The application never
// depends on a particular algorithm such as bcrypt or argon2.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}
