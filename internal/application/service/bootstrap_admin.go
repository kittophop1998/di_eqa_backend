package service

import (
	"context"
	"errors"
	"log"
	"strings"
	"unicode/utf8"

	applicationport "github.com/di-eqa/backend/internal/application/port"
	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
)

// MinBootstrapPasswordLen is the minimum length of ADMIN_INITIAL_PASSWORD (A-02).
const MinBootstrapPasswordLen = 12

// AdminBootstrap creates the first super_admin from configuration (A-02).
type AdminBootstrap struct {
	users     port.UserRepository
	passwords applicationport.PasswordHasher
	clock     applicationport.Clock
}

func NewAdminBootstrap(u port.UserRepository, p applicationport.PasswordHasher, c applicationport.Clock) *AdminBootstrap {
	return &AdminBootstrap{users: u, passwords: p, clock: c}
}

// EnsureSuperAdmin creates one super_admin when none exists and credentials
// are supplied. It never modifies or deletes existing data. It reports whether
// a user was created.
func (b *AdminBootstrap) EnsureSuperAdmin(ctx context.Context, username, password string) (bool, error) {
	n, err := b.users.CountByRoleStatus(ctx, entity.RoleSuperAdmin, "")
	if err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" && password == "" {
		log.Printf("WARN: no super_admin exists and ADMIN_INITIAL_USERNAME/ADMIN_INITIAL_PASSWORD are not set; nothing was created")
		return false, nil
	}
	if !usernameRe.MatchString(username) {
		return false, errors.New("ADMIN_INITIAL_USERNAME must be 3-32 characters of a-z, 0-9, '.', '_' or '-'")
	}
	if utf8.RuneCountInString(password) < MinBootstrapPasswordLen || len(password) > maxPasswordLen {
		return false, errors.New("ADMIN_INITIAL_PASSWORD must be 12-72 characters")
	}
	hash, err := b.passwords.Hash(password)
	if err != nil {
		return false, err
	}
	_, err = b.users.Create(ctx, &entity.User{
		Username: username, UsernameLower: username,
		FullName:  "Super Administrator",
		Password:  hash,
		Role:      entity.RoleSuperAdmin,
		Status:    entity.UserActive,
		CreatedAt: b.clock.Now(),
	})
	if err != nil {
		return false, err
	}
	return true, nil
}
