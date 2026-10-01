package port

import "errors"

// Persistence errors that repositories translate driver errors into, so the
// application layer never depends on a database driver.
var (
	// ErrNotFound is returned when a document does not exist.
	ErrNotFound = errors.New("port: not found")
	// ErrDuplicate is returned when a unique index rejects a write.
	ErrDuplicate = errors.New("port: duplicate key")
	// ErrConflict is returned when a conditional (compare-and-set) write did
	// not match, e.g. the attempt was no longer in progress.
	ErrConflict = errors.New("port: conditional update did not match")
)

// Page is offset pagination input.
type Page struct {
	Page     int
	PageSize int
}

// Skip returns the number of documents to skip.
func (p Page) Skip() int64 { return int64((p.Page - 1) * p.PageSize) }

// Limit returns the page size.
func (p Page) Limit() int64 { return int64(p.PageSize) }
