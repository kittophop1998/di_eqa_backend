package service

import (
	"context"
	"strings"
	"time"

	"github.com/di-eqa/backend/internal/domain/entity"
	"github.com/di-eqa/backend/internal/domain/port"
	"github.com/di-eqa/backend/internal/infrastructure/config"
	"github.com/di-eqa/backend/internal/utils"
	"golang.org/x/crypto/bcrypt"
)

// AuthService handles user registration and login use cases.
type AuthService struct {
	users     port.UserRepository
	hospitals port.HospitalRepository
	audit     *AuditService
	cfg       *config.Config
}

func NewAuthService(u port.UserRepository, h port.HospitalRepository, audit *AuditService, cfg *config.Config) *AuthService {
	return &AuthService{users: u, hospitals: h, audit: audit, cfg: cfg}
}

// RegisterAddress mirrors entity.Address in the application layer.
type RegisterAddress struct {
	AddressNo   string
	Building    string
	SubDistrict string
	District    string
	Province    string
	PostalCode  string
}

// RegisterInput carries data needed to register a new user.
//
// MemberType drives the flow:
//   - "internal" (default): HospitalCode is required; user is bound to a hospital.
//   - "external"           : HospitalCode is ignored; user stands alone.
type RegisterInput struct {
	MemberType   string
	HospitalCode string

	Username string
	Password string

	FirstName string
	LastName  string
	FullName  string
	Email     string

	Clinic       string
	LabName      string
	HospitalType string
	BedSize      string

	Address RegisterAddress

	CertificateYear int

	// Request context fields used for audit logging. Optional.
	IP        string
	UserAgent string
}

// RegisterOutput carries the result of a successful registration.
type RegisterOutput struct {
	Token string
	User  entity.PublicUser
}

func (s *AuthService) Register(ctx context.Context, in RegisterInput) (*RegisterOutput, error) {
	memberType := normalizeMemberType(in.MemberType)
	if memberType == "" {
		return nil, ErrInvalidMemberType
	}

	username := strings.ToLower(strings.TrimSpace(in.Username))
	fullName := entity.ComposeFullName(in.FirstName, in.LastName, in.FullName)

	// Enforce global username uniqueness so login-without-hospitalCode works.
	count, err := s.users.CountByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrUsernameExists
	}

	profile := entity.Profile{
		MemberType:   memberType,
		FirstName:    strings.TrimSpace(in.FirstName),
		LastName:     strings.TrimSpace(in.LastName),
		Clinic:       strings.TrimSpace(in.Clinic),
		LabName:      strings.TrimSpace(in.LabName),
		HospitalType: strings.TrimSpace(in.HospitalType),
		BedSize:      strings.TrimSpace(in.BedSize),
		Address: entity.Address{
			AddressNo:   strings.TrimSpace(in.Address.AddressNo),
			Building:    strings.TrimSpace(in.Address.Building),
			SubDistrict: strings.TrimSpace(in.Address.SubDistrict),
			District:    strings.TrimSpace(in.Address.District),
			Province:    strings.TrimSpace(in.Address.Province),
			PostalCode:  strings.TrimSpace(in.Address.PostalCode),
		},
		CertificateYear: in.CertificateYear,
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &entity.User{
		Username:  username,
		FullName:  fullName,
		Email:     strings.TrimSpace(in.Email),
		Password:  string(hash),
		Role:      entity.RoleUser,
		Profile:   profile,
		CreatedAt: time.Now(),
	}

	var hospital *entity.Hospital

	if memberType == entity.MemberInternal {
		h, err := s.hospitals.FindByCode(ctx, strings.ToUpper(strings.TrimSpace(in.HospitalCode)))
		if err != nil {
			return nil, ErrHospitalNotFound
		}
		hospital = h
		user.HospitalID = hospital.ID
	}

	id, err := s.users.Create(ctx, user)
	if err != nil {
		return nil, err
	}
	user.ID = id

	hospitalIDHex := ""
	if hospital != nil {
		hospitalIDHex = hospital.ID.Hex()
	}
	token, err := utils.GenerateToken(s.cfg.JWTSecret, user.ID.Hex(), hospitalIDHex, user.Role, s.cfg.JWTExpiry)
	if err != nil {
		return nil, err
	}

	// Audit: self-registered users are both actor and target. Hospital info
	// (when present) is stored in metadata so the super-admin UI can show
	// where the new account belongs without an extra lookup.
	if s.audit != nil {
		meta := map[string]any{
			"memberType": memberType,
			"username":   user.Username,
		}
		if hospital != nil {
			meta["hospitalCode"] = hospital.Code
			meta["hospitalName"] = hospital.Name
		}
		s.audit.Record(ctx, entity.AuditActionUserRegister, RecordParams{
			ActorID:    user.ID,
			ActorName:  user.FullName,
			ActorRole:  user.Role,
			TargetID:   user.ID,
			TargetName: user.FullName,
			IP:         in.IP,
			UserAgent:  in.UserAgent,
			Metadata:   meta,
		})
	}

	return &RegisterOutput{Token: token, User: user.ToPublic(hospital)}, nil
}

// normalizeMemberType coerces a free-form member type string into a known
// constant, returning "" when the value is not recognised.
// An empty input is treated as "internal" for backward compatibility.
func normalizeMemberType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", entity.MemberInternal:
		return entity.MemberInternal
	case entity.MemberExternal:
		return entity.MemberExternal
	default:
		return ""
	}
}

// LoginInput carries credentials. HospitalCode is no longer required;
// login is now a global username + password lookup.
type LoginInput struct {
	Username string
	Password string
}

// LoginOutput carries the result of a successful login.
type LoginOutput struct {
	Token string
	User  entity.PublicUser
}

// Login performs a global username+password authentication — no hospitalCode needed.
// It resolves the user's hospital (if any) and embeds it in the public user struct.
func (s *AuthService) Login(ctx context.Context, in LoginInput) (*LoginOutput, error) {
	username := strings.ToLower(strings.TrimSpace(in.Username))

	user, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(in.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	var hospital *entity.Hospital
	if !user.HospitalID.IsZero() {
		hospital, _ = s.hospitals.FindByID(ctx, user.HospitalID)
	}

	hospitalIDHex := ""
	if !user.HospitalID.IsZero() {
		hospitalIDHex = user.HospitalID.Hex()
	}

	token, err := utils.GenerateToken(s.cfg.JWTSecret, user.ID.Hex(), hospitalIDHex, user.Role, s.cfg.JWTExpiry)
	if err != nil {
		return nil, err
	}
	return &LoginOutput{Token: token, User: user.ToPublic(hospital)}, nil
}

func (s *AuthService) Me(ctx context.Context, userIDHex string) (*entity.PublicUser, error) {
	id, err := parseOID(userIDHex)
	if err != nil {
		return nil, ErrNotFound
	}
	user, err := s.users.FindByID(ctx, id)
	if err != nil {
		return nil, ErrNotFound
	}
	var hospital *entity.Hospital
	if !user.HospitalID.IsZero() {
		hospital, _ = s.hospitals.FindByID(ctx, user.HospitalID)
	}
	pub := user.ToPublic(hospital)
	return &pub, nil
}
