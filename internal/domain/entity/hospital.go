package entity

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Hospital is a participating laboratory site. Its address fields mirror the
// naming used by entity.Address so both can be rendered by the same UI.
type Hospital struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Code        string             `bson:"code"          json:"code"`
	Name        string             `bson:"name"          json:"name"`
	Logo        string             `bson:"logo"          json:"logo"`
	Province    string             `bson:"province"      json:"province"`
	District    string             `bson:"district"      json:"district"`
	SubDistrict string             `bson:"subDistrict"   json:"subDistrict"`
	PostalCode  string             `bson:"postalCode"    json:"postalCode"`
	CreatedAt   time.Time          `bson:"createdAt"     json:"createdAt"`
}
