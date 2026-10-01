// Package security provides concrete security adapters for application ports.
package security

import (
	"errors"
	"time"

	applicationport "github.com/di-eqa/backend/internal/application/port"
	"github.com/golang-jwt/jwt/v5"
)

const jwtIssuer = "di-eqa"

// JWTService signs and verifies JWTs. Tokens carry only sub, iat and exp:
// role, status and hospital are never trusted from a token (BR-02).
type JWTService struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewJWTService(secret string, ttl time.Duration) *JWTService {
	return &JWTService{secret: []byte(secret), ttl: ttl, now: time.Now}
}

func (s *JWTService) Issue(identity applicationport.TokenClaims) (string, time.Time, error) {
	now := s.now()
	exp := now.Add(s.ttl)
	claims := jwt.RegisteredClaims{
		Subject:   identity.UserID,
		ExpiresAt: jwt.NewNumericDate(exp),
		IssuedAt:  jwt.NewNumericDate(now),
		Issuer:    jwtIssuer,
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	return tok, exp.UTC(), err
}

// Verify checks signature, algorithm, issuer and expiry. An expired but
// otherwise valid token yields applicationport.ErrTokenExpired.
func (s *JWTService) Verify(tokenString string) (applicationport.TokenClaims, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(tokenString, &claims,
		func(*jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(jwtIssuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(s.now),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return applicationport.TokenClaims{}, applicationport.ErrTokenExpired
		}
		return applicationport.TokenClaims{}, err
	}
	if claims.Subject == "" {
		return applicationport.TokenClaims{}, errors.New("token has no subject")
	}
	return applicationport.TokenClaims{UserID: claims.Subject}, nil
}
