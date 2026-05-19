package service

import (
	"context"
	"strings"

	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// UserService handles user-management use cases (list / change role).
type UserService struct {
	users     port.UserRepository
	hospitals port.HospitalRepository
	audit     *AuditService
}

func NewUserService(u port.UserRepository, h port.HospitalRepository, audit *AuditService) *UserService {
	return &UserService{users: u, hospitals: h, audit: audit}
}

// UserListItem is the DTO returned for each user in the list.
type UserListItem struct {
	ID          string          `json:"id"`
	Username    string          `json:"username"`
	FullName    string          `json:"fullName"`
	Email       string          `json:"email"`
	Role        string          `json:"role"`
	MemberType  string          `json:"memberType"`
	HospitalID  string          `json:"hospitalId,omitempty"`
	HospitalName string         `json:"hospitalName,omitempty"`
	Profile     entity.Profile  `json:"profile,omitempty"`
	CreatedAt   string          `json:"createdAt"`
}

// ListUsersInput carries pagination + search parameters.
type ListUsersInput struct {
	Search string
	Page   int64
	Limit  int64
}

// ListUsersOutput wraps paginated results.
type ListUsersOutput struct {
	Users []UserListItem `json:"users"`
	Total int64          `json:"total"`
	Page  int64          `json:"page"`
	Limit int64          `json:"limit"`
}

func (s *UserService) ListUsers(ctx context.Context, in ListUsersInput) (*ListUsersOutput, error) {
	if in.Limit <= 0 || in.Limit > 100 {
		in.Limit = 20
	}
	if in.Page <= 0 {
		in.Page = 1
	}
	skip := (in.Page - 1) * in.Limit

	users, total, err := s.users.ListAll(ctx, in.Search, skip, in.Limit)
	if err != nil {
		return nil, err
	}

	// Collect unique hospitalIDs for a single batch lookup.
	idSet := map[primitive.ObjectID]bool{}
	for _, u := range users {
		if !u.HospitalID.IsZero() {
			idSet[u.HospitalID] = true
		}
	}
	hospNames := map[primitive.ObjectID]string{}
	for hid := range idSet {
		if h, err := s.hospitals.FindByID(ctx, hid); err == nil {
			hospNames[hid] = h.Name
		}
	}

	items := make([]UserListItem, 0, len(users))
	for _, u := range users {
		item := UserListItem{
			ID:         u.ID.Hex(),
			Username:   u.Username,
			FullName:   u.FullName,
			Email:      u.Email,
			Role:       u.Role,
			MemberType: u.Profile.MemberType,
			Profile:    u.Profile,
			CreatedAt:  u.CreatedAt.Format("2006-01-02"),
		}
		if !u.HospitalID.IsZero() {
			item.HospitalID = u.HospitalID.Hex()
			item.HospitalName = hospNames[u.HospitalID]
		}
		items = append(items, item)
	}

	return &ListUsersOutput{
		Users: items,
		Total: total,
		Page:  in.Page,
		Limit: in.Limit,
	}, nil
}

// allowedRoles is the set of roles that can be assigned via the API.
var allowedRoles = map[string]bool{
	entity.RoleUser:       true,
	entity.RoleAdmin:      true,
	entity.RoleSuperAdmin: true,
}

// UpdateRoleInput carries the payload for a role change.
type UpdateRoleInput struct {
	UserIDHex   string // the user whose role is being changed
	CallerID    string // the user id (hex) of the admin making the request
	CallerRole  string // the role of the user making the request
	NewRole     string
	IP          string
	UserAgent   string
}

func (s *UserService) UpdateRole(ctx context.Context, in UpdateRoleInput) error {
	newRole := strings.TrimSpace(in.NewRole)
	if !allowedRoles[newRole] {
		return ErrForbidden
	}
	// Only super_admin can grant or revoke super_admin.
	if in.CallerRole != entity.RoleSuperAdmin && (newRole == entity.RoleSuperAdmin || in.NewRole == entity.RoleSuperAdmin) {
		return ErrForbidden
	}
	id, err := parseOID(in.UserIDHex)
	if err != nil {
		return ErrNotFound
	}

	// Load the target before mutation so we can capture the previous role and
	// a human-friendly name in the audit log.
	target, err := s.users.FindByID(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	oldRole := target.Role
	if oldRole == newRole {
		// No-op: still record success but skip the DB write below.
		return nil
	}

	if err := s.users.UpdateRole(ctx, id, newRole); err != nil {
		return err
	}

	if s.audit != nil {
		actorID, _ := primitive.ObjectIDFromHex(in.CallerID)
		actorName := ""
		if !actorID.IsZero() {
			if actor, err := s.users.FindByID(ctx, actorID); err == nil {
				actorName = actor.FullName
			}
		}
		s.audit.Record(ctx, entity.AuditActionUserRoleChange, RecordParams{
			ActorID:    actorID,
			ActorName:  actorName,
			ActorRole:  in.CallerRole,
			TargetID:   target.ID,
			TargetName: target.FullName,
			IP:         in.IP,
			UserAgent:  in.UserAgent,
			Metadata: map[string]any{
				"oldRole":  oldRole,
				"newRole":  newRole,
				"username": target.Username,
			},
		})
	}
	return nil
}
