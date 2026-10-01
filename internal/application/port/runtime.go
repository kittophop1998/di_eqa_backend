package port

import (
	"context"
	"time"
)

// Clock abstracts time so lifecycle rules are testable.
type Clock interface {
	Now() time.Time
}

// Randomness supplies the secure randomness the use cases need.
type Randomness interface {
	// Intn returns a uniform integer in [0,n) from a CSPRNG.
	Intn(n int) int
	// QuestionID returns a fresh opaque id (>= 8 chars) that is not derived
	// from any image id (BR-13).
	QuestionID() (string, error)
	// CertNo returns a fresh certificate number (BR-37).
	CertNo(now time.Time) (string, error)
}

// Image is the raw bytes of a stored cell image.
type Image struct {
	Data        []byte
	ContentType string
}

// ImageStore loads image bytes for server-side streaming (BR-14).
type ImageStore interface {
	Get(ctx context.Context, path string) (*Image, error)
	// PreviewURL returns a URL an admin browser may use to preview the image,
	// or "" when none is available.
	PreviewURL(path string) string
}
