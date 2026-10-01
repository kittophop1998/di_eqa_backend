package service

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	applicationport "github.com/di-eqa/backend/internal/application/port"
	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// AdminUserService implements the admin user-management use cases (C-02).
type AdminUserService struct {
	users     port.UserRepository
	hospitals port.HospitalRepository
	audit     *AuditService
	clock     applicationport.Clock
}

func NewAdminUserService(u port.UserRepository, h port.HospitalRepository, audit *AuditService, clock applicationport.Clock) *AdminUserService {
	return &AdminUserService{users: u, hospitals: h, audit: audit, clock: clock}
}

// AdminUser is the admin view of a user (§5.6).
type AdminUser struct {
	ID                string         `json:"id"`
	Username          string         `json:"username"`
	FullName          string         `json:"fullName"`
	Email             *string        `json:"email"`
	Role              entity.Role    `json:"role"`
	Status            string         `json:"status"`
	Hospital          *HospitalRef   `json:"hospital"`
	RequestedHospital *HospitalRef   `json:"requestedHospital"`
	Profile           entity.Profile `json:"profile"`
	CreatedAt         time.Time      `json:"createdAt"`
	ApprovedAt        *time.Time     `json:"approvedAt"`
	ApprovedBy        *UserRef       `json:"approvedBy"`
}

// ListUsersInput drives GET /admin/users.
type ListUsersInput struct {
	Status     string
	HospitalID string
	Role       string
	Query      string
	Page       int
	PageSize   int
}

func (s *AdminUserService) List(ctx context.Context, in ListUsersInput) (*Paged[AdminUser], error) {
	var f port.UserFilter
	var issues []entity.FieldIssue
	if in.Status != "" {
		if st := entity.UserStatus(in.Status); st.Valid() {
			f.Status = st
		} else {
			issues = append(issues, entity.FieldIssue{Field: "status", Issue: "unknown status"})
		}
	}
	if in.Role != "" {
		if r := entity.Role(in.Role); r.Valid() {
			f.Role = r
		} else {
			issues = append(issues, entity.FieldIssue{Field: "role", Issue: "unknown role"})
		}
	}
	if in.HospitalID != "" {
		if id, err := parseOID(in.HospitalID); err == nil {
			f.HospitalID = id
		} else {
			issues = append(issues, entity.FieldIssue{Field: "hospitalId", Issue: "invalid id"})
		}
	}
	if len(issues) > 0 {
		return nil, validation(issues...)
	}
	f.Query = strings.TrimSpace(in.Query)

	page, size := normalizePage(in.Page, in.PageSize)
	users, total, err := s.users.List(ctx, f, port.Page{Page: page, PageSize: size})
	if err != nil {
		return nil, internal(err)
	}
	items, err := s.decorate(ctx, users)
	if err != nil {
		return nil, err
	}
	return &Paged[AdminUser]{Items: items, Page: page, PageSize: size, Total: total}, nil
}

// Approve activates a pending/rejected user and sets the confirmed hospital (BR-42).
func (s *AdminUserService) Approve(ctx context.Context, idHex, hospitalIDHex string, actor Actor) (*AdminUser, error) {
	id, err := parseOID(idHex)
	if err != nil {
		return nil, notFound()
	}
	if strings.TrimSpace(hospitalIDHex) == "" {
		return nil, validationOne("hospitalId", "is required")
	}
	hid, err := parseOID(hospitalIDHex)
	if err != nil {
		return nil, validationOne("hospitalId", "invalid id")
	}
	h, active, err := loadActiveHospital(ctx, s.hospitals, hid)
	if err != nil {
		return nil, err
	}
	if h == nil || !active {
		return nil, validationOne("hospitalId", "hospital not found or inactive")
	}
	if _, err := s.users.FindByID(ctx, id); err != nil {
		return nil, mapNotFound(err)
	}
	now := s.clock.Now()
	st := entity.UserActive
	u, err := s.users.Update(ctx, id, port.UserUpdate{
		Status: &st, HospitalID: &hid, ApprovedAt: &now, ApprovedBy: &actor.ID, ClearRejectReason: true,
	}, []entity.UserStatus{entity.UserPending, entity.UserRejected})
	if err != nil {
		if errors.Is(err, port.ErrConflict) {
			return nil, invalidState("ผู้ใช้นี้ไม่ได้อยู่ในสถานะรออนุมัติ")
		}
		return nil, mapNotFound(err)
	}
	s.audit.Log(ctx, entity.AuditActionUserApprove, actor, u.ID, u.FullName,
		map[string]any{"username": u.Username, "hospitalId": hid.Hex(), "hospitalName": h.Name})
	return s.one(ctx, u)
}

// Reject marks a pending user rejected (BR-42).
func (s *AdminUserService) Reject(ctx context.Context, idHex, reason string, actor Actor) (*AdminUser, error) {
	id, err := parseOID(idHex)
	if err != nil {
		return nil, notFound()
	}
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) > 500 {
		return nil, validationOne("reason", "must be at most 500 characters")
	}
	if _, err := s.users.FindByID(ctx, id); err != nil {
		return nil, mapNotFound(err)
	}
	st := entity.UserRejected
	upd := port.UserUpdate{Status: &st}
	if reason != "" {
		upd.RejectReason = &reason
	}
	u, err := s.users.Update(ctx, id, upd, []entity.UserStatus{entity.UserPending})
	if err != nil {
		if errors.Is(err, port.ErrConflict) {
			return nil, invalidState("ผู้ใช้นี้ไม่ได้อยู่ในสถานะรออนุมัติ")
		}
		return nil, mapNotFound(err)
	}
	s.audit.Log(ctx, entity.AuditActionUserReject, actor, u.ID, u.FullName,
		map[string]any{"username": u.Username, "reason": reason})
	return s.one(ctx, u)
}

// PatchUserInput is a partial user update; role changes are not allowed here.
type PatchUserInput struct {
	HospitalID *string
	Status     *string
	FullName   *string
	Email      *string
}

// Patch updates hospital, status (active/disabled), name and email.
func (s *AdminUserService) Patch(ctx context.Context, idHex string, in PatchUserInput, actor Actor) (*AdminUser, error) {
	id, err := parseOID(idHex)
	if err != nil {
		return nil, notFound()
	}
	target, err := s.users.FindByID(ctx, id)
	if err != nil {
		return nil, mapNotFound(err)
	}
	// A non-super admin may not touch a super_admin account (BR-06).
	if target.Role.IsSuper() && !actor.Role.IsSuper() {
		return nil, forbidden()
	}

	var upd port.UserUpdate
	var issues []entity.FieldIssue
	var casStatus []entity.UserStatus
	changed := false
	finalHospital := target.HospitalID

	if in.FullName != nil {
		n := strings.TrimSpace(*in.FullName)
		if n == "" || len([]rune(n)) > 200 {
			issues = append(issues, entity.FieldIssue{Field: "fullName", Issue: "must be 1-200 characters"})
		}
		upd.FullName, changed = &n, true
	}
	if in.Email != nil {
		e := strings.TrimSpace(*in.Email)
		if e != "" {
			if a, err := mail.ParseAddress(e); err != nil || a.Address != e || len(e) > 254 {
				issues = append(issues, entity.FieldIssue{Field: "email", Issue: "is not a valid email address"})
			}
		}
		upd.Email, changed = &e, true
	}
	var hospitalName string
	if in.HospitalID != nil {
		hid, err := parseOID(*in.HospitalID)
		if err != nil {
			issues = append(issues, entity.FieldIssue{Field: "hospitalId", Issue: "invalid id"})
		} else {
			h, active, lerr := loadActiveHospital(ctx, s.hospitals, hid)
			if lerr != nil {
				return nil, lerr
			}
			if h == nil || !active {
				issues = append(issues, entity.FieldIssue{Field: "hospitalId", Issue: "hospital not found or inactive"})
			} else {
				upd.HospitalID, changed, finalHospital, hospitalName = &hid, true, hid, h.Name
			}
		}
	}
	if in.Status != nil {
		st := entity.UserStatus(*in.Status)
		switch st {
		case entity.UserActive:
			if target.Status != entity.UserActive && target.Status != entity.UserDisabled {
				return nil, invalidState("ใช้การอนุมัติสำหรับผู้ใช้ที่ยังไม่ได้รับอนุมัติ")
			}
			if target.Role == entity.RoleUser && finalHospital.IsZero() {
				issues = append(issues, entity.FieldIssue{Field: "hospitalId", Issue: "required to activate a user"})
			}
		case entity.UserDisabled:
			if target.Role.IsSuper() && target.Status == entity.UserActive {
				n, err := s.users.CountByRoleStatus(ctx, entity.RoleSuperAdmin, entity.UserActive)
				if err != nil {
					return nil, internal(err)
				}
				if n <= 1 {
					return nil, newErr(CodeLastSuperAdmin, "ไม่สามารถระงับ super_admin คนสุดท้ายได้", nil)
				}
			}
		default:
			issues = append(issues, entity.FieldIssue{Field: "status", Issue: "must be active or disabled"})
		}
		if st != target.Status {
			upd.Status, changed = &st, true
			casStatus = []entity.UserStatus{target.Status}
		}
	}
	if len(issues) > 0 {
		return nil, validation(issues...)
	}
	if !changed {
		if in.FullName == nil && in.Email == nil && in.HospitalID == nil && in.Status == nil {
			return nil, validationOne("body", "at least one field is required")
		}
		return s.one(ctx, target)
	}

	u, err := s.users.Update(ctx, id, upd, casStatus)
	if err != nil {
		if errors.Is(err, port.ErrConflict) {
			return nil, invalidState("สถานะผู้ใช้เปลี่ยนไปแล้ว กรุณาลองใหม่")
		}
		return nil, mapNotFound(err)
	}
	meta := map[string]any{"username": u.Username}
	if upd.Status != nil {
		meta["status"] = string(*upd.Status)
	}
	if upd.HospitalID != nil {
		meta["hospitalId"], meta["hospitalName"] = upd.HospitalID.Hex(), hospitalName
	}
	s.audit.Log(ctx, entity.AuditActionUserUpdate, actor, u.ID, u.FullName, meta)
	return s.one(ctx, u)
}

// SetRole changes a user's role. Super_admin only; never on oneself; never
// removing the last active super_admin (BR-03, BR-06).
func (s *AdminUserService) SetRole(ctx context.Context, idHex, roleStr string, actor Actor) (*AdminUser, error) {
	if !actor.Role.IsSuper() {
		return nil, forbidden()
	}
	id, err := parseOID(idHex)
	if err != nil {
		return nil, notFound()
	}
	role := entity.Role(roleStr)
	if !role.Valid() {
		return nil, validationOne("role", "must be user, admin or super_admin")
	}
	if id == actor.ID {
		return nil, forbidden()
	}
	target, err := s.users.FindByID(ctx, id)
	if err != nil {
		return nil, mapNotFound(err)
	}
	if target.Role == role {
		return s.one(ctx, target)
	}
	if target.Role.IsSuper() && target.Status == entity.UserActive {
		n, err := s.users.CountByRoleStatus(ctx, entity.RoleSuperAdmin, entity.UserActive)
		if err != nil {
			return nil, internal(err)
		}
		if n <= 1 {
			return nil, newErr(CodeLastSuperAdmin, "ไม่สามารถลดสิทธิ์ super_admin คนสุดท้ายได้", nil)
		}
	}
	u, err := s.users.Update(ctx, id, port.UserUpdate{Role: &role}, nil)
	if err != nil {
		return nil, mapNotFound(err)
	}
	s.audit.Log(ctx, entity.AuditActionUserRoleChange, actor, u.ID, u.FullName,
		map[string]any{"oldRole": string(target.Role), "newRole": string(role), "username": u.Username})
	return s.one(ctx, u)
}

func (s *AdminUserService) one(ctx context.Context, u *entity.User) (*AdminUser, error) {
	items, err := s.decorate(ctx, []entity.User{*u})
	if err != nil {
		return nil, err
	}
	return &items[0], nil
}

// decorate resolves hospital and approver references in two batched queries.
func (s *AdminUserService) decorate(ctx context.Context, users []entity.User) ([]AdminUser, error) {
	hospSet := map[primitive.ObjectID]bool{}
	apprSet := map[primitive.ObjectID]bool{}
	for i := range users {
		if !users[i].HospitalID.IsZero() {
			hospSet[users[i].HospitalID] = true
		}
		if !users[i].RequestedHospitalID.IsZero() {
			hospSet[users[i].RequestedHospitalID] = true
		}
		if !users[i].ApprovedBy.IsZero() {
			apprSet[users[i].ApprovedBy] = true
		}
	}
	hospitals := map[primitive.ObjectID]*entity.Hospital{}
	if len(hospSet) > 0 {
		list, err := s.hospitals.FindByIDs(ctx, keys(hospSet))
		if err != nil {
			return nil, internal(err)
		}
		for i := range list {
			hospitals[list[i].ID] = &list[i]
		}
	}
	approvers := map[primitive.ObjectID]*entity.User{}
	if len(apprSet) > 0 {
		list, err := s.users.FindByIDs(ctx, keys(apprSet))
		if err != nil {
			return nil, internal(err)
		}
		for i := range list {
			approvers[list[i].ID] = &list[i]
		}
	}
	out := make([]AdminUser, 0, len(users))
	for i := range users {
		u := &users[i]
		au := AdminUser{
			ID: u.ID.Hex(), Username: u.Username, FullName: u.FullName, Role: u.Role,
			Status: string(u.Status), Profile: u.Profile, CreatedAt: u.CreatedAt, ApprovedAt: u.ApprovedAt,
			Hospital: hospitalRef(hospitals[u.HospitalID]), RequestedHospital: hospitalRef(hospitals[u.RequestedHospitalID]),
		}
		if u.Email != "" {
			e := u.Email
			au.Email = &e
		}
		if a := approvers[u.ApprovedBy]; a != nil {
			au.ApprovedBy = &UserRef{ID: a.ID.Hex(), FullName: a.FullName}
		}
		out = append(out, au)
	}
	return out, nil
}

func keys(m map[primitive.ObjectID]bool) []primitive.ObjectID {
	out := make([]primitive.ObjectID, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
