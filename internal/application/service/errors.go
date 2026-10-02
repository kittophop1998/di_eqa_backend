package service

import (
	"errors"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Common domain errors used across all services.
var (
	ErrNotFound           = errors.New("not found")
	ErrBadInput           = errors.New("invalid input")
	ErrHospitalNotFound   = errors.New("hospital not found")
	ErrHospitalCodeExists = errors.New("hospital code already exists")
	ErrHospitalInUse      = errors.New("hospital still has members")
	ErrInvalidPostalCode  = errors.New("invalid postal code")
	ErrUsernameExists     = errors.New("username already exists in this hospital")
	ErrUsernameTaken      = errors.New("username already taken")
	ErrInvalidMemberType  = errors.New("invalid member type")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrForbidden          = errors.New("forbidden")
	ErrSessionNotRunning  = errors.New("session has not started")
	ErrDuplicateSubmit    = errors.New("submission already in progress")
)

// parseOID is a helper to parse a hex string into ObjectID.
func parseOID(hex string) (primitive.ObjectID, error) {
	return primitive.ObjectIDFromHex(hex)
}
