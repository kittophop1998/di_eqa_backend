package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Hospital struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Code      string             `bson:"code" json:"code"`
	Name      string             `bson:"name" json:"name"`
	Logo      string             `bson:"logo" json:"logo"`
	Province  string             `bson:"province" json:"province"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}
