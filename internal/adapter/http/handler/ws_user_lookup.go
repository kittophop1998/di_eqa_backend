package handler

import (
	"context"
	"fmt"

	"github.com/di-eqa/backend/internal/domain/port"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// wsUserLookup implements the userLookup interface using repository ports.
type wsUserLookup struct {
	users     port.UserRepository
	hospitals port.HospitalRepository
}

// NewWSUserLookup creates a userLookup backed by the given repositories.
func NewWSUserLookup(u port.UserRepository, h port.HospitalRepository) *wsUserLookup {
	return &wsUserLookup{users: u, hospitals: h}
}

func (w *wsUserLookup) FindByID(ctx context.Context, id primitive.ObjectID) (username, fullName, hospitalName string, err error) {
	user, err := w.users.FindByID(ctx, id)
	if err != nil {
		return "", "", "", fmt.Errorf("user not found")
	}
	var hospName string
	if !user.HospitalID.IsZero() {
		if h, e := w.hospitals.FindByID(ctx, user.HospitalID); e == nil {
			hospName = h.Name
		}
	}
	return user.Username, user.FullName, hospName, nil
}
