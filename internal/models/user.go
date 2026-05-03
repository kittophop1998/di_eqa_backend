package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	RoleUser      = "user"
	RoleInstructor = "instructor"
	RoleAdmin     = "admin"
)

type User struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	HospitalID primitive.ObjectID `bson:"hospitalId" json:"hospitalId"`
	Username   string             `bson:"username" json:"username"`
	FullName   string             `bson:"fullName" json:"fullName"`
	Email      string             `bson:"email" json:"email"`
	Password   string             `bson:"password" json:"-"`
	Role       string             `bson:"role" json:"role"`
	CreatedAt  time.Time          `bson:"createdAt" json:"createdAt"`
}

type PublicUser struct {
	ID         primitive.ObjectID `json:"id"`
	HospitalID primitive.ObjectID `json:"hospitalId"`
	Username   string             `json:"username"`
	FullName   string             `json:"fullName"`
	Email      string             `json:"email"`
	Role       string             `json:"role"`
	Hospital   *Hospital          `json:"hospital,omitempty"`
}

func (u *User) ToPublic(h *Hospital) PublicUser {
	return PublicUser{
		ID:         u.ID,
		HospitalID: u.HospitalID,
		Username:   u.Username,
		FullName:   u.FullName,
		Email:      u.Email,
		Role:       u.Role,
		Hospital:   h,
	}
}
