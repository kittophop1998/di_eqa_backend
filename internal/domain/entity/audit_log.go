package entity

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// AuditLog actions — keep these short, machine-friendly and stable so that
// querying or filtering by action stays simple.
const (
	AuditActionUserRegister   = "user.register"
	AuditActionUserRoleChange = "user.role_change"
)

// AuditLog is an immutable record of a security/governance-relevant event.
// It is appended once and never updated; the collection is meant to be
// read-only from the application's perspective.
type AuditLog struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"        json:"id"`
	Action     string             `bson:"action"               json:"action"`
	ActorID    primitive.ObjectID `bson:"actorId,omitempty"    json:"actorId,omitempty"`
	ActorName  string             `bson:"actorName,omitempty"  json:"actorName,omitempty"`
	ActorRole  string             `bson:"actorRole,omitempty"  json:"actorRole,omitempty"`
	TargetID   primitive.ObjectID `bson:"targetId,omitempty"   json:"targetId,omitempty"`
	TargetName string             `bson:"targetName,omitempty" json:"targetName,omitempty"`
	IP         string             `bson:"ip,omitempty"         json:"ip,omitempty"`
	UserAgent  string             `bson:"userAgent,omitempty"  json:"userAgent,omitempty"`
	// Metadata holds free-form, action-specific details (e.g. oldRole, newRole,
	// hospitalCode, memberType). Kept as a map to avoid schema churn.
	Metadata  map[string]any `bson:"metadata,omitempty" json:"metadata,omitempty"`
	CreatedAt time.Time      `bson:"createdAt"          json:"createdAt"`
}
