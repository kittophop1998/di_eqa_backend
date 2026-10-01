package entity

// Role is an account role (BR-01). Only the three constants below are valid.
type Role string

const (
	RoleUser       Role = "user"
	RoleAdmin      Role = "admin"
	RoleSuperAdmin Role = "super_admin"
)

// Valid reports whether r is one of the known roles. Unknown values (for
// example the retired "instructor") are invalid and must be rejected.
func (r Role) Valid() bool {
	switch r {
	case RoleUser, RoleAdmin, RoleSuperAdmin:
		return true
	}
	return false
}

// IsStaff is the single place that decides whether a role may use the admin
// surface (BR-03).
func (r Role) IsStaff() bool { return r == RoleAdmin || r == RoleSuperAdmin }

// IsSuper reports whether r is super_admin.
func (r Role) IsSuper() bool { return r == RoleSuperAdmin }

// UserStatus is the lifecycle state of an account (D2).
type UserStatus string

const (
	UserPending  UserStatus = "pending"
	UserActive   UserStatus = "active"
	UserRejected UserStatus = "rejected"
	UserDisabled UserStatus = "disabled"
)

// Valid reports whether s is a known status.
func (s UserStatus) Valid() bool {
	switch s {
	case UserPending, UserActive, UserRejected, UserDisabled:
		return true
	}
	return false
}
