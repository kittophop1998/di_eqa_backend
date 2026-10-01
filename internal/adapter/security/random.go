package security

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"time"

	"github.com/di-eqa/backend/internal/domain/entity"
)

// CryptoRandomness implements application/port.Randomness with crypto/rand.
type CryptoRandomness struct{}

func NewCryptoRandomness() *CryptoRandomness { return &CryptoRandomness{} }

// Intn returns a uniform integer in [0,n). It panics only if the OS random
// source fails, which is unrecoverable for a security-sensitive draw.
func (CryptoRandomness) Intn(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic("security: crypto/rand failed: " + err.Error())
	}
	return int(v.Int64())
}

// QuestionID returns 24 hex characters of pure randomness (BR-13).
func (CryptoRandomness) QuestionID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// CertNo returns a fresh DIEQA-{YYYY}-{8 chars} number (BR-37).
func (CryptoRandomness) CertNo(now time.Time) (string, error) {
	return entity.NewCertNo(now, rand.Reader)
}

// SystemClock is the production Clock: UTC, millisecond precision (MongoDB's
// resolution), so values round-trip through the database unchanged.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }
