package entity

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Hospital is a participating laboratory site. Active=false is a soft delete.
type Hospital struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Code        string             `bson:"code"          json:"code"`
	Name        string             `bson:"name"          json:"name"`
	Logo        string             `bson:"logo"          json:"logo"`
	Province    string             `bson:"province"      json:"province"`
	District    string             `bson:"district"      json:"district"`
	SubDistrict string             `bson:"subDistrict"   json:"subDistrict"`
	PostalCode  string             `bson:"postalCode"    json:"postalCode"`
	Active      bool               `bson:"active"        json:"active"`
	CreatedAt   time.Time          `bson:"createdAt"     json:"createdAt"`
}
