package entity

import (
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
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
type Profile struct {
	FirstName       string  `bson:"firstName,omitempty"       json:"firstName,omitempty"`
	LastName        string  `bson:"lastName,omitempty"        json:"lastName,omitempty"`
	Clinic          string  `bson:"clinic,omitempty"          json:"clinic,omitempty"`
	LabName         string  `bson:"labName,omitempty"         json:"labName,omitempty"`
	HospitalType    string  `bson:"hospitalType,omitempty"    json:"hospitalType,omitempty"`
	BedSize         string  `bson:"bedSize,omitempty"         json:"bedSize,omitempty"`
	Address         Address `bson:"address,omitempty"         json:"address,omitempty"`
	CertificateYear int     `bson:"certificateYear,omitempty" json:"certificateYear,omitempty"`
}

// User is an account. Role, Status and HospitalID stored here are the only
// source of truth for authorization (BR-02, BR-04).
type User struct {
	ID                  primitive.ObjectID `bson:"_id,omitempty"                 json:"id"`
	HospitalID          primitive.ObjectID `bson:"hospitalId,omitempty"          json:"hospitalId,omitempty"`
	RequestedHospitalID primitive.ObjectID `bson:"requestedHospitalId,omitempty" json:"requestedHospitalId,omitempty"`
	Username            string             `bson:"username"                      json:"username"`
	UsernameLower       string             `bson:"usernameLower"                 json:"-"`
	FullName            string             `bson:"fullName"                      json:"fullName"`
	Email               string             `bson:"email"                         json:"email"`
	Password            string             `bson:"password"                      json:"-"`
	Role                Role               `bson:"role"                          json:"role"`
	Status              UserStatus         `bson:"status"                        json:"status"`
	Profile             Profile            `bson:"profile,omitempty"             json:"profile,omitempty"`
	RejectReason        string             `bson:"rejectReason,omitempty"        json:"-"`
	ApprovedAt          *time.Time         `bson:"approvedAt,omitempty"          json:"approvedAt,omitempty"`
	ApprovedBy          primitive.ObjectID `bson:"approvedBy,omitempty"          json:"-"`
	CreatedAt           time.Time          `bson:"createdAt"                     json:"createdAt"`
}

// CanLogin reports whether the account may authenticate at all.
func (u *User) CanLogin() bool { return u.Status == UserActive }

// ComposeFullName builds a non-empty full name from first/last name parts,
// falling back to the provided fallback when both are empty.
func ComposeFullName(first, last, fallback string) string {
	name := strings.TrimSpace(strings.TrimSpace(first) + " " + strings.TrimSpace(last))
	if name != "" {
		return name
	}
	return strings.TrimSpace(fallback)
}
