// Package security provides concrete security adapters for application ports.
package security

import (
	"errors"
	"time"

	applicationport "github.com/di-eqa/backend/internal/application/port"
	"github.com/golang-jwt/jwt/v5"
)

// JWTService signs and verifies JWTs. It is an adapter: neither domain nor
// application code needs to know the JWT library or signing algorithm.
type JWTService struct {
	secret string
	ttl    time.Duration
}

func NewJWTService(secret string, ttl time.Duration) *JWTService {
	return &JWTService{secret: secret, ttl: ttl}
}

func (s *JWTService) Issue(identity applicationport.TokenClaims) (string, error) {
	claims := claims{
		UserID:     identity.UserID,
		HospitalID: identity.HospitalID,
		Role:       identity.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "di-eqa",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.secret))
}

// Verify is used by the HTTP authentication adapter before a request enters a
// use case. Invalid or unsigned tokens are rejected here.
func (s *JWTService) Verify(tokenString string) (applicationport.TokenClaims, error) {
	parsed, err := jwt.ParseWithClaims(tokenString, &claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("invalid signing method")
		}
		return []byte(s.secret), nil
	})
	if err != nil {
		return applicationport.TokenClaims{}, err
	}
	jwtClaims, ok := parsed.Claims.(*claims)
	if !ok || !parsed.Valid {
		return applicationport.TokenClaims{}, errors.New("invalid token")
	}
	return applicationport.TokenClaims{UserID: jwtClaims.UserID, HospitalID: jwtClaims.HospitalID, Role: jwtClaims.Role}, nil
}

type claims struct {
	UserID     string `json:"uid"`
	HospitalID string `json:"hid"`
	Role       string `json:"role"`
	jwt.RegisteredClaims
}
