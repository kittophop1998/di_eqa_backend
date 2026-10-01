// Package draw holds the pure question-draw rules (BR-11, BR-13).
package draw

import (
	"errors"
	"fmt"
)

// ErrPoolTooSmall is returned when the pool has fewer items than requested.
var ErrPoolTooSmall = errors.New("draw: pool smaller than requested count")

// Intn returns a uniform random integer in [0,n). Production code passes a
// CSPRNG-backed implementation; tests pass a deterministic one.
type Intn func(n int) int

// Sample returns n distinct items chosen uniformly at random from pool, in
// random order (partial Fisher-Yates). The input slice is not modified.
// Pure random: no stratification by type, so an unbalanced pool still yields
// exactly n distinct items.
func Sample[T any](pool []T, n int, intn Intn) ([]T, error) {
	if n < 0 {
		return nil, fmt.Errorf("draw: negative count %d", n)
	}
	if n > len(pool) {
		return nil, ErrPoolTooSmall
	}
	work := make([]T, len(pool))
	copy(work, pool)
	for i := 0; i < n; i++ {
		j := i + intn(len(work)-i)
		work[i], work[j] = work[j], work[i]
	}
	return work[:n:n], nil
}

// UniqueIDs returns n distinct opaque ids produced by gen. It fails after too
// many collisions so a broken generator cannot loop forever.
func UniqueIDs(n int, gen func() (string, error)) ([]string, error) {
	seen := make(map[string]struct{}, n)
	out := make([]string, 0, n)
	for attempts := 0; len(out) < n; attempts++ {
		if attempts > n*10+10 {
			return nil, errors.New("draw: id generator keeps colliding")
		}
		id, err := gen()
		if err != nil {
			return nil, err
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}
