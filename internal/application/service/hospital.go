package service

import (
	"context"
	"strings"
	"time"

	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// HospitalService handles hospital query and management use cases.
type HospitalService struct {
	hospitals port.HospitalRepository
	users     port.UserRepository
	audit     *AuditService
}

func NewHospitalService(h port.HospitalRepository, u port.UserRepository, audit *AuditService) *HospitalService {
	return &HospitalService{hospitals: h, users: u, audit: audit}
}

func (s *HospitalService) List(ctx context.Context, query string) ([]entity.Hospital, error) {
	return s.hospitals.List(ctx, query)
}

func (s *HospitalService) GetByCode(ctx context.Context, code string) (*entity.Hospital, error) {
	h, err := s.hospitals.FindByCode(ctx, code)
	if err != nil {
		return nil, ErrNotFound
	}
	return h, nil
}

// HospitalFields carries the editable attributes of a hospital. Shared by the
// create and update use cases so both validate identically.
type HospitalFields struct {
	Code        string
	Name        string
	Logo        string
	Province    string
	District    string
	SubDistrict string
	PostalCode  string
}

// Actor identifies the authenticated caller for audit records.
type Actor struct {
	ID        string
	Role      string
	IP        string
	UserAgent string
}

// normalise trims every field and upper-cases the code so lookups by code stay
// case-insensitive, then validates the ones with a required format.
func (f *HospitalFields) normalise() error {
	f.Code = strings.ToUpper(strings.TrimSpace(f.Code))
	f.Name = strings.TrimSpace(f.Name)
	f.Logo = strings.TrimSpace(f.Logo)
	f.Province = strings.TrimSpace(f.Province)
	f.District = strings.TrimSpace(f.District)
	f.SubDistrict = strings.TrimSpace(f.SubDistrict)
	f.PostalCode = strings.TrimSpace(f.PostalCode)

	if f.Code == "" || f.Name == "" {
		return ErrBadInput
	}
	if f.PostalCode != "" && !isThaiPostalCode(f.PostalCode) {
		return ErrInvalidPostalCode
	}
	return nil
}

// isThaiPostalCode reports whether s is exactly five ASCII digits.
func isThaiPostalCode(s string) bool {
	if len(s) != 5 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (s *HospitalService) Create(ctx context.Context, in HospitalFields, actor Actor) (*entity.Hospital, error) {
	if err := in.normalise(); err != nil {
		return nil, err
	}
	if existing, err := s.hospitals.FindByCode(ctx, in.Code); err == nil && existing != nil {
		return nil, ErrHospitalCodeExists
	}

	h := &entity.Hospital{
		Code:        in.Code,
		Name:        in.Name,
		Logo:        in.Logo,
		Province:    in.Province,
		District:    in.District,
		SubDistrict: in.SubDistrict,
		PostalCode:  in.PostalCode,
		CreatedAt:   time.Now(),
	}
	id, err := s.hospitals.Create(ctx, h)
	if err != nil {
		return nil, err
	}
	h.ID = id

	s.recordAudit(ctx, entity.AuditActionHospitalCreate, actor, h, map[string]any{
		"code": h.Code,
		"name": h.Name,
	})
	return h, nil
}

func (s *HospitalService) Update(ctx context.Context, idHex string, in HospitalFields, actor Actor) (*entity.Hospital, error) {
	if err := in.normalise(); err != nil {
		return nil, err
	}
	id, err := parseOID(idHex)
	if err != nil {
		return nil, ErrNotFound
	}
	current, err := s.hospitals.FindByID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	// The code must stay unique — reject it only when another document owns it.
	if other, err := s.hospitals.FindByCode(ctx, in.Code); err == nil && other != nil && other.ID != id {
		return nil, ErrHospitalCodeExists
	}

	updated := &entity.Hospital{
		ID:          id,
		Code:        in.Code,
		Name:        in.Name,
		Logo:        in.Logo,
		Province:    in.Province,
		District:    in.District,
		SubDistrict: in.SubDistrict,
		PostalCode:  in.PostalCode,
		CreatedAt:   current.CreatedAt,
	}
	if err := s.hospitals.Update(ctx, id, updated); err != nil {
		return nil, err
	}

	s.recordAudit(ctx, entity.AuditActionHospitalUpdate, actor, updated, map[string]any{
		"code":    updated.Code,
		"name":    updated.Name,
		"oldCode": current.Code,
		"oldName": current.Name,
	})
	return updated, nil
}

func (s *HospitalService) Delete(ctx context.Context, idHex string, actor Actor) error {
	id, err := parseOID(idHex)
	if err != nil {
		return ErrNotFound
	}
	current, err := s.hospitals.FindByID(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	// Users carry a hospitalId reference; deleting the hospital would orphan
	// them, so the caller has to move or remove those members first.
	members, err := s.users.CountByHospital(ctx, id)
	if err != nil {
		return err
	}
	if members > 0 {
		return ErrHospitalInUse
	}
	if err := s.hospitals.Delete(ctx, id); err != nil {
		return err
	}

	s.recordAudit(ctx, entity.AuditActionHospitalDelete, actor, current, map[string]any{
		"code": current.Code,
		"name": current.Name,
	})
	return nil
}

// recordAudit resolves the actor's display name and appends an audit entry.
func (s *HospitalService) recordAudit(
	ctx context.Context,
	action string,
	actor Actor,
	target *entity.Hospital,
	metadata map[string]any,
) {
	if s.audit == nil {
		return
	}
	actorID, _ := primitive.ObjectIDFromHex(actor.ID)
	actorName := ""
	if !actorID.IsZero() {
		if u, err := s.users.FindByID(ctx, actorID); err == nil {
			actorName = u.FullName
		}
	}
	s.audit.Record(ctx, action, RecordParams{
		ActorID:    actorID,
		ActorName:  actorName,
		ActorRole:  actor.Role,
		TargetID:   target.ID,
		TargetName: target.Name,
		IP:         actor.IP,
		UserAgent:  actor.UserAgent,
		Metadata:   metadata,
	})
}
