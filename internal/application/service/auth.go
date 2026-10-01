package service

import (
	"context"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	applicationport "github.com/di-eqa/backend/internal/application/port"
	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
)

var usernameRe = regexp.MustCompile(`^[a-z0-9._-]{3,32}$`)

const (
	minPasswordLen = 8
	maxPasswordLen = 72 // bcrypt input limit
)

// AuthService handles registration, login and per-request authentication.
type AuthService struct {
	users     port.UserRepository
	hospitals port.HospitalRepository
	audit     *AuditService
	tokens    applicationport.TokenIssuer
	passwords applicationport.PasswordHasher
	clock     applicationport.Clock
}

func NewAuthService(
	u port.UserRepository,
	h port.HospitalRepository,
	audit *AuditService,
	tokens applicationport.TokenIssuer,
	passwords applicationport.PasswordHasher,
	clock applicationport.Clock,
) *AuthService {
	return &AuthService{users: u, hospitals: h, audit: audit, tokens: tokens, passwords: passwords, clock: clock}
}

// ProfileInput is the optional registration profile (§5.4).
type ProfileInput struct {
	Clinic          string
	LabName         string
	HospitalType    string
	BedSize         string
	Address         entity.Address
	CertificateYear int
}

// RegisterInput carries the self-registration request. Role, status and
// hospital are deliberately absent: the client cannot choose them (BR-40).
type RegisterInput struct {
	Username            string
	Password            string
	FirstName           string
	LastName            string
	Email               string
	RequestedHospitalID string
	Profile             ProfileInput

	IP        string
	UserAgent string
}

// Register creates a pending user. It issues no token (D2).
func (s *AuthService) Register(ctx context.Context, in RegisterInput) error {
	username := strings.ToLower(strings.TrimSpace(in.Username))
	first := strings.TrimSpace(in.FirstName)
	last := strings.TrimSpace(in.LastName)
	email := strings.TrimSpace(in.Email)

	var issues []entity.FieldIssue
	if !usernameRe.MatchString(username) {
		issues = append(issues, entity.FieldIssue{Field: "username", Issue: "must be 3-32 characters of a-z, 0-9, '.', '_' or '-'"})
	}
	if utf8.RuneCountInString(in.Password) < minPasswordLen {
		issues = append(issues, entity.FieldIssue{Field: "password", Issue: "must be at least 8 characters"})
	} else if len(in.Password) > maxPasswordLen {
		issues = append(issues, entity.FieldIssue{Field: "password", Issue: "must be at most 72 bytes"})
	}
	if first == "" {
		issues = append(issues, entity.FieldIssue{Field: "firstName", Issue: "is required"})
	}
	if last == "" {
		issues = append(issues, entity.FieldIssue{Field: "lastName", Issue: "is required"})
	}
	if utf8.RuneCountInString(first) > 100 || utf8.RuneCountInString(last) > 100 {
		issues = append(issues, entity.FieldIssue{Field: "firstName", Issue: "name is too long"})
	}
	if email != "" {
		if a, err := mail.ParseAddress(email); err != nil || a.Address != email || len(email) > 254 {
			issues = append(issues, entity.FieldIssue{Field: "email", Issue: "is not a valid email address"})
		}
	}
	var hospital *entity.Hospital
	hid, err := parseOID(in.RequestedHospitalID)
	if err != nil {
		issues = append(issues, entity.FieldIssue{Field: "requestedHospitalId", Issue: "must be a valid hospital id"})
	} else if h, err := s.hospitals.FindByID(ctx, hid); err != nil || !h.Active {
		if err != nil && !errors.Is(err, port.ErrNotFound) {
			return internal(err)
		}
		issues = append(issues, entity.FieldIssue{Field: "requestedHospitalId", Issue: "hospital not found or inactive"})
	} else {
		hospital = h
	}
	if len(issues) > 0 {
		return validation(issues...)
	}

	hash, err := s.passwords.Hash(in.Password)
	if err != nil {
		return internal(err)
	}
	p := in.Profile
	user := &entity.User{
		Username:            username,
		UsernameLower:       username,
		FullName:            entity.ComposeFullName(first, last, username),
		Email:               email,
		Password:            hash,
		Role:                entity.RoleUser, // never taken from the client
		Status:              entity.UserPending,
		RequestedHospitalID: hospital.ID,
		Profile: entity.Profile{
			FirstName:       first,
			LastName:        last,
			Clinic:          strings.TrimSpace(p.Clinic),
			LabName:         strings.TrimSpace(p.LabName),
			HospitalType:    strings.TrimSpace(p.HospitalType),
			BedSize:         strings.TrimSpace(p.BedSize),
			Address:         p.Address,
			CertificateYear: p.CertificateYear,
		},
		CreatedAt: s.clock.Now(),
	}
	id, err := s.users.Create(ctx, user)
	if err != nil {
		if errors.Is(err, port.ErrDuplicate) {
			return newErr(CodeUsernameTaken, "ชื่อผู้ใช้นี้ถูกใช้งานแล้ว กรุณาเลือกชื่อใหม่", nil)
		}
		return internal(err)
	}
	user.ID = id

	s.audit.Record(ctx, entity.AuditActionUserRegister, RecordParams{
		ActorID: id, ActorName: user.FullName, ActorRole: string(user.Role),
		TargetID: id, TargetName: user.FullName,
		IP: in.IP, UserAgent: in.UserAgent,
		Metadata: map[string]any{"username": username, "requestedHospitalId": hospital.ID.Hex()},
	})
	return nil
}

// Me is the authenticated user's profile (§5.3).
type Me struct {
	ID       string       `json:"id"`
	Username string       `json:"username"`
	FullName string       `json:"fullName"`
	Email    *string      `json:"email"`
	Role     entity.Role  `json:"role"`
	Status   string       `json:"status"`
	Hospital *HospitalRef `json:"hospital"`
}

// LoginInput carries credentials.
type LoginInput struct {
	Username string
	Password string
}

// LoginOutput is the successful login result.
type LoginOutput struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
	User      Me        `json:"user"`
}

// Login authenticates by username+password, then gates on account status
// (BR-41). No token is issued for a non-active account.
func (s *AuthService) Login(ctx context.Context, in LoginInput) (*LoginOutput, error) {
	username := strings.ToLower(strings.TrimSpace(in.Username))
	if username == "" || in.Password == "" {
		return nil, validation(entity.FieldIssue{Field: "username", Issue: "username and password are required"})
	}
	invalid := newErr(CodeInvalidCredentials, "ชื่อผู้ใช้หรือรหัสผ่านไม่ถูกต้อง", nil)

	user, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, port.ErrNotFound) {
			return nil, invalid
		}
		return nil, internal(err)
	}
	if err := s.passwords.Compare(user.Password, in.Password); err != nil {
		return nil, invalid
	}
	switch user.Status {
	case entity.UserActive:
	case entity.UserPending:
		return nil, newErr(CodeAccountPending, "บัญชีของคุณอยู่ระหว่างรอผู้ดูแลระบบอนุมัติ", nil)
	case entity.UserRejected:
		return nil, newErr(CodeAccountRejected, "คำขอสมัครของคุณไม่ได้รับการอนุมัติ", nil)
	default:
		return nil, newErr(CodeAccountDisabled, "บัญชีของคุณถูกระงับการใช้งาน", nil)
	}
	if !user.Role.Valid() {
		return nil, forbidden()
	}

	token, exp, err := s.tokens.Issue(applicationport.TokenClaims{UserID: user.ID.Hex()})
	if err != nil {
		return nil, internal(err)
	}
	me, err := s.toMe(ctx, user)
	if err != nil {
		return nil, err
	}
	return &LoginOutput{Token: token, ExpiresAt: exp.UTC(), User: *me}, nil
}

// Authenticate resolves a token subject to a Principal using the database as
// the only source of role, status and hospital (BR-02).
func (s *AuthService) Authenticate(ctx context.Context, userIDHex string) (*Principal, error) {
	unauth := newErr(CodeUnauthenticated, "กรุณาเข้าสู่ระบบ", nil)
	id, err := parseOID(userIDHex)
	if err != nil {
		return nil, unauth
	}
	user, err := s.users.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, port.ErrNotFound) {
			return nil, unauth
		}
		return nil, internal(err)
	}
	if !user.CanLogin() || !user.Role.Valid() {
		return nil, unauth
	}
	return &Principal{
		UserID: user.ID, Username: user.Username, FullName: user.FullName,
		Role: user.Role, Status: user.Status, HospitalID: user.HospitalID,
	}, nil
}

// Me returns the profile of an authenticated principal.
func (s *AuthService) Me(ctx context.Context, p *Principal) (*Me, error) {
	user, err := s.users.FindByID(ctx, p.UserID)
	if err != nil {
		if errors.Is(err, port.ErrNotFound) {
			return nil, newErr(CodeUnauthenticated, "กรุณาเข้าสู่ระบบ", nil)
		}
		return nil, internal(err)
	}
	return s.toMe(ctx, user)
}

func (s *AuthService) toMe(ctx context.Context, u *entity.User) (*Me, error) {
	me := &Me{
		ID: u.ID.Hex(), Username: u.Username, FullName: u.FullName,
		Role: u.Role, Status: string(entity.UserActive),
	}
	if u.Email != "" {
		e := u.Email
		me.Email = &e
	}
	if !u.HospitalID.IsZero() {
		h, err := s.hospitals.FindByID(ctx, u.HospitalID)
		if err != nil && !errors.Is(err, port.ErrNotFound) {
			return nil, internal(err)
		}
		if err == nil {
			me.Hospital = hospitalRef(h)
		}
	}
	return me, nil
}
