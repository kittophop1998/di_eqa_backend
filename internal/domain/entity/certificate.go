package entity

import (
	"fmt"
	"io"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Certificate is issued once per (quiz,user) when an attempt passes (BR-36).
// All display fields are snapshots (BR-38).
type Certificate struct {
	ID            primitive.ObjectID `bson:"_id,omitempty"`
	CertNo        string             `bson:"certNo"`
	AttemptID     primitive.ObjectID `bson:"attemptId"`
	QuizID        primitive.ObjectID `bson:"quizId"`
	UserID        primitive.ObjectID `bson:"userId"`
	HospitalID    primitive.ObjectID `bson:"hospitalId"`
	RecipientName string             `bson:"recipientName"`
	HospitalName  string             `bson:"hospitalName"`
	QuizTitle     string             `bson:"quizTitle"`
	Percent       float64            `bson:"percent"`
	IssuedAt      time.Time          `bson:"issuedAt"`
	RevokedAt     *time.Time         `bson:"revokedAt,omitempty"`
	RevokeReason  string             `bson:"revokeReason,omitempty"`
}

// Revoked reports whether the certificate has been revoked.
func (c *Certificate) Revoked() bool { return c.RevokedAt != nil }

// crockford is the Crockford base32 alphabet (no I, L, O, U).
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewCertNo builds "DIEQA-{YYYY}-{8 Crockford base32}" using r as the random
// source (crypto/rand.Reader in production) (BR-37).
func NewCertNo(now time.Time, r io.Reader) (string, error) {
	var b [8]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return "", err
	}
	out := make([]byte, 8)
	for i, v := range b {
		out[i] = crockford[v&31] // 256 % 32 == 0, so this is unbiased
	}
	return fmt.Sprintf("DIEQA-%04d-%s", now.UTC().Year(), out), nil
}
