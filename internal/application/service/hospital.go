package service

import (
	"context"
	"errors"
	"strings"
	"time"

	applicationport "github.com/di-eqa/backend/internal/application/port"
	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// HospitalService handles the public hospital list and admin hospital management.
type HospitalService struct {
	hospitals   port.HospitalRepository
	users       port.UserRepository
	assignments port.AssignmentRepository
	audit       *AuditService
	clock       applicationport.Clock
}

func NewHospitalService(
	h port.HospitalRepository, u port.UserRepository, a port.AssignmentRepository,
	audit *AuditService, clock applicationport.Clock,
) *HospitalService {
	return &HospitalService{hospitals: h, users: u, assignments: a, audit: audit, clock: clock}
}

// ListPublic returns active hospitals as {id,name} only; codes are never
// exposed (BR-43).
func (s *HospitalService) ListPublic(ctx context.Context) ([]HospitalRef, error) {
	list, err := s.hospitals.ListActive(ctx)
	if err != nil {
		return nil, internal(err)
	}
	out := make([]HospitalRef, 0, len(list))
	for i := range list {
		out = append(out, HospitalRef{ID: list[i].ID.Hex(), Name: list[i].Name})
	}
	return out, nil
}

// AdminHospital is the admin view of a hospital (§5.6).
type AdminHospital struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Province    string    `json:"province"`
	District    string    `json:"district"`
	SubDistrict string    `json:"subDistrict"`
	PostalCode  string    `json:"postalCode"`
	Logo        string    `json:"logo"`
	Active      bool      `json:"active"`
	UserCount   int64     `json:"userCount"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ListHospitalsInput drives the admin list.
type ListHospitalsInput struct {
	Query    string
	Active   *bool
	Page     int
	PageSize int
}

func (s *HospitalService) List(ctx context.Context, in ListHospitalsInput) (*Paged[AdminHospital], error) {
	page, size := normalizePage(in.Page, in.PageSize)
	list, total, err := s.hospitals.List(ctx,
		port.HospitalFilter{Query: strings.TrimSpace(in.Query), Active: in.Active},
		port.Page{Page: page, PageSize: size})
	if err != nil {
		return nil, internal(err)
	}
	items := make([]AdminHospital, 0, len(list))
	for i := range list {
		n, err := s.users.CountByHospital(ctx, list[i].ID)
		if err != nil {
			return nil, internal(err)
		}
		items = append(items, toAdminHospital(&list[i], n))
	}
	return &Paged[AdminHospital]{Items: items, Page: page, PageSize: size, Total: total}, nil
}

func toAdminHospital(h *entity.Hospital, users int64) AdminHospital {
	return AdminHospital{
		ID: h.ID.Hex(), Code: h.Code, Name: h.Name, Province: h.Province,
		District: h.District, SubDistrict: h.SubDistrict, PostalCode: h.PostalCode,
		Logo: h.Logo, Active: h.Active, UserCount: users, CreatedAt: h.CreatedAt,
	}
}

// HospitalFields carries the editable attributes of a hospital.
type HospitalFields struct {
	Code        string
	Name        string
	Logo        string
	Province    string
	District    string
	SubDistrict string
	PostalCode  string
	Active      *bool // update only; nil keeps the current value
}

func (f *HospitalFields) normalise() *Error {
	f.Code = strings.ToUpper(strings.TrimSpace(f.Code))
	f.Name = strings.TrimSpace(f.Name)
	f.Logo = strings.TrimSpace(f.Logo)
	f.Province = strings.TrimSpace(f.Province)
	f.District = strings.TrimSpace(f.District)
	f.SubDistrict = strings.TrimSpace(f.SubDistrict)
	f.PostalCode = strings.TrimSpace(f.PostalCode)

	var issues []entity.FieldIssue
	if f.Code == "" {
		issues = append(issues, entity.FieldIssue{Field: "code", Issue: "is required"})
	}
	if f.Name == "" {
		issues = append(issues, entity.FieldIssue{Field: "name", Issue: "is required"})
	}
	if f.PostalCode != "" && !isThaiPostalCode(f.PostalCode) {
		issues = append(issues, entity.FieldIssue{Field: "postalCode", Issue: "must be 5 digits"})
	}
	if len(issues) > 0 {
		return validation(issues...)
	}
	return nil
}

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

func duplicateField(field string) *Error {
	return newErr(CodeDuplicate, "ข้อมูลนี้ถูกใช้งานแล้ว", map[string]any{"field": field})
}

func (s *HospitalService) Create(ctx context.Context, in HospitalFields, actor Actor) (*AdminHospital, error) {
	if e := in.normalise(); e != nil {
		return nil, e
	}
	h := &entity.Hospital{
		Code: in.Code, Name: in.Name, Logo: in.Logo, Province: in.Province,
		District: in.District, SubDistrict: in.SubDistrict, PostalCode: in.PostalCode,
		Active: true, CreatedAt: s.clock.Now(),
	}
	id, err := s.hospitals.Create(ctx, h)
	if err != nil {
		if errors.Is(err, port.ErrDuplicate) {
			return nil, duplicateField("code")
		}
		return nil, internal(err)
	}
	h.ID = id
	s.audit.Log(ctx, entity.AuditActionHospitalCreate, actor, id, h.Name, map[string]any{"code": h.Code})
	out := toAdminHospital(h, 0)
	return &out, nil
}

func (s *HospitalService) Update(ctx context.Context, idHex string, in HospitalFields, actor Actor) (*AdminHospital, error) {
	if e := in.normalise(); e != nil {
		return nil, e
	}
	id, err := parseOID(idHex)
	if err != nil {
		return nil, notFound()
	}
	cur, err := s.hospitals.FindByID(ctx, id)
	if err != nil {
		return nil, mapNotFound(err)
	}
	updated := *cur
	updated.Code, updated.Name, updated.Logo = in.Code, in.Name, in.Logo
	updated.Province, updated.District, updated.SubDistrict, updated.PostalCode = in.Province, in.District, in.SubDistrict, in.PostalCode
	if in.Active != nil {
		updated.Active = *in.Active
	}
	if err := s.hospitals.Update(ctx, id, &updated); err != nil {
		if errors.Is(err, port.ErrDuplicate) {
			return nil, duplicateField("code")
		}
		return nil, mapNotFound(err)
	}
	s.audit.Log(ctx, entity.AuditActionHospitalUpdate, actor, id, updated.Name,
		map[string]any{"code": updated.Code, "oldCode": cur.Code, "oldName": cur.Name})
	n, err := s.users.CountByHospital(ctx, id)
	if err != nil {
		return nil, internal(err)
	}
	out := toAdminHospital(&updated, n)
	return &out, nil
}

// Delete soft-deletes (active=false) a hospital that has no users or
// assignments (§5.6).
func (s *HospitalService) Delete(ctx context.Context, idHex string, actor Actor) error {
	id, err := parseOID(idHex)
	if err != nil {
		return notFound()
	}
	cur, err := s.hospitals.FindByID(ctx, id)
	if err != nil {
		return mapNotFound(err)
	}
	members, err := s.users.CountByHospital(ctx, id)
	if err != nil {
		return internal(err)
	}
	as, err := s.assignments.ListByHospital(ctx, id)
	if err != nil {
		return internal(err)
	}
	if members > 0 || len(as) > 0 {
		return newErr(CodeHospitalInUse, "ลบไม่ได้ ยังมีผู้ใช้หรือแบบทดสอบที่ผูกกับโรงพยาบาลนี้อยู่",
			map[string]any{"users": members, "assignments": len(as)})
	}
	if err := s.hospitals.SetActive(ctx, id, false); err != nil {
		return mapNotFound(err)
	}
	s.audit.Log(ctx, entity.AuditActionHospitalDelete, actor, id, cur.Name, map[string]any{"code": cur.Code})
	return nil
}

// mapNotFound converts a repository error into an application error.
func mapNotFound(err error) error {
	if errors.Is(err, port.ErrNotFound) {
		return notFound()
	}
	return internal(err)
}

// activeHospital loads a hospital and requires it to be active.
func loadActiveHospital(ctx context.Context, repo port.HospitalRepository, id primitive.ObjectID) (*entity.Hospital, bool, error) {
	h, err := repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, port.ErrNotFound) {
			return nil, false, nil
		}
		return nil, false, internal(err)
	}
	return h, h.Active, nil
}
