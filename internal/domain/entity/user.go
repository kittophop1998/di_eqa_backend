package entity

import (
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	RoleUser       = "user"
	RoleInstructor = "instructor" // kept for backward-compat with seeded data; treat as admin
	RoleAdmin      = "admin"
	RoleSuperAdmin = "super_admin"
)

// Member types — distinguishes internal hospital staff from external personnel.
const (
	MemberInternal = "internal" // บุคลากรภายใน (สังกัดโรงพยาบาลในระบบ)
	MemberExternal = "external" // บุคลากรภายนอก (ไม่สังกัดโรงพยาบาลในระบบ)
)

// Address is the document-delivery address (ที่อยู่สำหรับจัดส่งเอกสาร).
type Address struct {
	AddressNo   string `bson:"addressNo,omitempty"   json:"addressNo,omitempty"`
	Building    string `bson:"building,omitempty"    json:"building,omitempty"`
	SubDistrict string `bson:"subDistrict,omitempty" json:"subDistrict,omitempty"`
	District    string `bson:"district,omitempty"    json:"district,omitempty"`
	Province    string `bson:"province,omitempty"    json:"province,omitempty"`
	PostalCode  string `bson:"postalCode,omitempty"  json:"postalCode,omitempty"`
}

// Profile holds the extended membership data captured at registration.
// Stored as an embedded document so the User schema stays flat for legacy
// readers (FullName / Email) while new fields are grouped logically.
type Profile struct {
	MemberType      string  `bson:"memberType,omitempty"      json:"memberType,omitempty"`
	FirstName       string  `bson:"firstName,omitempty"       json:"firstName,omitempty"`
	LastName        string  `bson:"lastName,omitempty"        json:"lastName,omitempty"`
	Clinic          string  `bson:"clinic,omitempty"          json:"clinic,omitempty"`
	LabName         string  `bson:"labName,omitempty"         json:"labName,omitempty"`
	HospitalType    string  `bson:"hospitalType,omitempty"    json:"hospitalType,omitempty"`
	BedSize         string  `bson:"bedSize,omitempty"         json:"bedSize,omitempty"`
	Address         Address `bson:"address,omitempty"         json:"address,omitempty"`
	CertificateYear int     `bson:"certificateYear,omitempty" json:"certificateYear,omitempty"`
}

type User struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"        json:"id"`
	HospitalID primitive.ObjectID `bson:"hospitalId,omitempty" json:"hospitalId,omitempty"`
	Username   string             `bson:"username"             json:"username"`
	FullName   string             `bson:"fullName"             json:"fullName"`
	Email      string             `bson:"email"                json:"email"`
	Password   string             `bson:"password"             json:"-"`
	Role       string             `bson:"role"                 json:"role"`
	Profile    Profile            `bson:"profile,omitempty"    json:"profile,omitempty"`
	CreatedAt  time.Time          `bson:"createdAt"            json:"createdAt"`
}

type PublicUser struct {
	ID         primitive.ObjectID `json:"id"`
	HospitalID primitive.ObjectID `json:"hospitalId,omitempty"`
	Username   string             `json:"username"`
	FullName   string             `json:"fullName"`
	Email      string             `json:"email"`
	Role       string             `json:"role"`
	Profile    Profile            `json:"profile,omitempty"`
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
		Profile:    u.Profile,
		Hospital:   h,
	}
}

// ComposeFullName builds a non-empty full name from first/last name parts,
// falling back to the provided fallback when both are empty.
func ComposeFullName(first, last, fallback string) string {
	name := strings.TrimSpace(strings.TrimSpace(first) + " " + strings.TrimSpace(last))
	if name != "" {
		return name
	}
	return strings.TrimSpace(fallback)
}
