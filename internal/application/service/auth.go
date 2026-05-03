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
	cfg       *config.Config
}

func NewAuthService(u port.UserRepository, h port.HospitalRepository, cfg *config.Config) *AuthService {
	return &AuthService{users: u, hospitals: h, cfg: cfg}
}

// RegisterInput carries data needed to register a new user.
type RegisterInput struct {
	HospitalCode string
	Username     string
	FullName     string
	Email        string
	Password     string
}

// RegisterOutput carries the result of a successful registration.
type RegisterOutput struct {
	Token string
	User  entity.PublicUser
}

func (s *AuthService) Register(ctx context.Context, in RegisterInput) (*RegisterOutput, error) {
	hospital, err := s.hospitals.FindByCode(ctx, strings.ToUpper(in.HospitalCode))
	if err != nil {
		return nil, ErrHospitalNotFound
	}

	username := strings.ToLower(strings.TrimSpace(in.Username))
	count, err := s.users.CountByUsernameAndHospital(ctx, username, hospital.ID)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrUsernameExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &entity.User{
		HospitalID: hospital.ID,
		Username:   username,
		FullName:   strings.TrimSpace(in.FullName),
		Email:      strings.TrimSpace(in.Email),
		Password:   string(hash),
		Role:       entity.RoleUser,
		CreatedAt:  time.Now(),
	}
	id, err := s.users.Create(ctx, user)
	if err != nil {
		return nil, err
	}
	user.ID = id

	token, err := utils.GenerateToken(s.cfg.JWTSecret, user.ID.Hex(), hospital.ID.Hex(), user.Role, s.cfg.JWTExpiry)
	if err != nil {
		return nil, err
	}
	return &RegisterOutput{Token: token, User: user.ToPublic(hospital)}, nil
}

// LoginInput carries data needed to authenticate.
type LoginInput struct {
	HospitalCode string
	Username     string
	Password     string
	IsAdmin      bool
}

// LoginOutput carries the result of a successful login.
type LoginOutput struct {
	Token string
	User  entity.PublicUser
}

func (s *AuthService) Login(ctx context.Context, in LoginInput) (*LoginOutput, error) {
	username := strings.ToLower(strings.TrimSpace(in.Username))

	if in.IsAdmin || in.HospitalCode == "" {
		user, err := s.users.FindAdminByUsername(ctx, username)
		if err != nil {
			return nil, ErrInvalidCredentials
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(in.Password)); err != nil {
			return nil, ErrInvalidCredentials
		}
		token, err := utils.GenerateToken(s.cfg.JWTSecret, user.ID.Hex(), "", user.Role, s.cfg.JWTExpiry)
		if err != nil {
			return nil, err
		}
		return &LoginOutput{Token: token, User: user.ToPublic(nil)}, nil
	}

	hospital, err := s.hospitals.FindByCode(ctx, strings.ToUpper(in.HospitalCode))
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	user, err := s.users.FindByUsernameAndHospital(ctx, username, hospital.ID)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(in.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	token, err := utils.GenerateToken(s.cfg.JWTSecret, user.ID.Hex(), hospital.ID.Hex(), user.Role, s.cfg.JWTExpiry)
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
