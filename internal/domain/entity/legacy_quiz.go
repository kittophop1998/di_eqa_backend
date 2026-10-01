package entity

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// LegacyQuiz is the pre-v2 "cells[]" quiz document. It is kept only so old
// data can be read until Epic G removes the legacy session code. It never
// appears in the v2 API (B-02).
type LegacyQuiz struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title       string             `bson:"title"         json:"title"`
	Description string             `bson:"description"   json:"description"`
	Category    string             `bson:"category"      json:"category"`
	Categories  []LegacyCategory   `bson:"categories"    json:"categories"`
	Cells       []LegacyCell       `bson:"cells"         json:"cells"`
	PassPercent int                `bson:"passPercent"   json:"passPercent"`
	DurationSec int                `bson:"durationSec"   json:"durationSec"`
	CreatedBy   primitive.ObjectID `bson:"createdBy"     json:"createdBy"`
	CreatedAt   time.Time          `bson:"createdAt"     json:"createdAt"`
}

// LegacyCategory is a legacy answer category (migrated to CellType).
type LegacyCategory struct {
	Key   string `bson:"key"   json:"key"`
	Label string `bson:"label" json:"label"`
	Color string `bson:"color" json:"color"`
}

// LegacyCell is a legacy embedded cell image.
type LegacyCell struct {
	ID          string             `bson:"id"                json:"id"`
	AssetID     primitive.ObjectID `bson:"assetId,omitempty" json:"assetId,omitempty"`
	Path        string             `bson:"path,omitempty"    json:"path,omitempty"`
	CorrectType string             `bson:"correctType"       json:"correctType,omitempty"`
}
