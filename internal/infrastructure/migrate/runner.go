// Package migrate is a small, driver-free migration runner plus the concrete
// MongoDB steps. Migrations are versioned, idempotent, recorded in
// schema_migrations, and fail loudly; they never delete user data (A-04).
package migrate

import (
	"context"
	"fmt"
	"sort"
)

// Step is one migration. Up must be idempotent: re-running it on data that is
// already migrated must be a no-op, because a crash between Up and Record makes
// the step run again.
type Step struct {
	Version int
	Name    string
	Up      func(ctx context.Context) error
}

// Store records which versions have been applied.
type Store interface {
	Applied(ctx context.Context) (map[int]bool, error)
	Record(ctx context.Context, version int, name string) error
}

func sorted(steps []Step) []Step {
	out := append([]Step(nil), steps...)
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out
}

// Pending returns the steps that have not been applied yet, in version order.
func Pending(ctx context.Context, store Store, steps []Step) ([]Step, error) {
	applied, err := store.Applied(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: read applied versions: %w", err)
	}
	var out []Step
	for _, s := range sorted(steps) {
		if !applied[s.Version] {
			out = append(out, s)
		}
	}
	return out, nil
}

// Run applies every pending step in order and stops at the first failure,
// returning the versions applied so far and an error naming the failed step.
func Run(ctx context.Context, store Store, steps []Step, logf func(format string, args ...any)) ([]int, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	pending, err := Pending(ctx, store, steps)
	if err != nil {
		return nil, err
	}
	var done []int
	for _, s := range pending {
		logf("migration %d (%s): applying", s.Version, s.Name)
		if err := s.Up(ctx); err != nil {
			return done, fmt.Errorf("migration %d (%s) failed: %w", s.Version, s.Name, err)
		}
		if err := store.Record(ctx, s.Version, s.Name); err != nil {
			return done, fmt.Errorf("migration %d (%s) applied but could not be recorded: %w", s.Version, s.Name, err)
		}
		logf("migration %d (%s): done", s.Version, s.Name)
		done = append(done, s.Version)
	}
	return done, nil
}
