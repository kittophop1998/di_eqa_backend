package service

import (
	"context"
	"log"
	"time"

	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// AuditService records and queries security-relevant events.
//
// Recording is best-effort: callers should treat audit logging as a side
// effect that must never break the main flow. The Record* methods therefore
// swallow errors after logging them, so an outage in the audit collection
// never prevents user-facing operations from completing.
type AuditService struct {
	repo port.AuditLogRepository
}

func NewAuditService(repo port.AuditLogRepository) *AuditService {
	return &AuditService{repo: repo}
}

// RecordParams is the actor/context info captured for every audit event.
type RecordParams struct {
	ActorID    primitive.ObjectID
	ActorName  string
	ActorRole  string
	TargetID   primitive.ObjectID
	TargetName string
	IP         string
	UserAgent  string
	Metadata   map[string]any
}

// Record persists an audit log entry. Failures are logged but not returned —
// audit recording must never break the calling business flow.
func (s *AuditService) Record(ctx context.Context, action string, p RecordParams) {
	if s == nil || s.repo == nil {
		return
	}
	entry := &entity.AuditLog{
		Action:     action,
		ActorID:    p.ActorID,
		ActorName:  p.ActorName,
		ActorRole:  p.ActorRole,
		TargetID:   p.TargetID,
		TargetName: p.TargetName,
		IP:         p.IP,
		UserAgent:  p.UserAgent,
		Metadata:   p.Metadata,
		CreatedAt:  time.Now(),
	}
	if _, err := s.repo.Create(ctx, entry); err != nil {
		log.Printf("audit: failed to record %q: %v", action, err)
	}
}

// AuditLogItem is the DTO returned to API clients.
type AuditLogItem struct {
	ID         string         `json:"id"`
	Action     string         `json:"action"`
	ActorID    string         `json:"actorId,omitempty"`
	ActorName  string         `json:"actorName,omitempty"`
	ActorRole  string         `json:"actorRole,omitempty"`
	TargetID   string         `json:"targetId,omitempty"`
	TargetName string         `json:"targetName,omitempty"`
	IP         string         `json:"ip,omitempty"`
	UserAgent  string         `json:"userAgent,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  string         `json:"createdAt"`
}

// ListAuditLogsInput drives pagination + filtering.
type ListAuditLogsInput struct {
	Action string
	Page   int64
	Limit  int64
}

// ListAuditLogsOutput wraps paginated results.
type ListAuditLogsOutput struct {
	Logs  []AuditLogItem `json:"logs"`
	Total int64          `json:"total"`
	Page  int64          `json:"page"`
	Limit int64          `json:"limit"`
}

func (s *AuditService) List(ctx context.Context, in ListAuditLogsInput) (*ListAuditLogsOutput, error) {
	if in.Limit <= 0 || in.Limit > 200 {
		in.Limit = 50
	}
	if in.Page <= 0 {
		in.Page = 1
	}
	skip := (in.Page - 1) * in.Limit

	logs, total, err := s.repo.List(ctx, in.Action, skip, in.Limit)
	if err != nil {
		return nil, err
	}

	items := make([]AuditLogItem, 0, len(logs))
	for _, l := range logs {
		item := AuditLogItem{
			ID:         l.ID.Hex(),
			Action:     l.Action,
			ActorName:  l.ActorName,
			ActorRole:  l.ActorRole,
			TargetName: l.TargetName,
			IP:         l.IP,
			UserAgent:  l.UserAgent,
			Metadata:   l.Metadata,
			CreatedAt:  l.CreatedAt.UTC().Format(time.RFC3339),
		}
		if !l.ActorID.IsZero() {
			item.ActorID = l.ActorID.Hex()
		}
		if !l.TargetID.IsZero() {
			item.TargetID = l.TargetID.Hex()
		}
		items = append(items, item)
	}

	return &ListAuditLogsOutput{
		Logs:  items,
		Total: total,
		Page:  in.Page,
		Limit: in.Limit,
	}, nil
}
