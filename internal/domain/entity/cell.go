package entity

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CellType is a classification answer option (e.g. "neutrophil").
type CellType struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"-"`
	Key       string             `bson:"key"           json:"key"`
	Label     string             `bson:"label"         json:"label"`
	SortOrder int                `bson:"sortOrder"     json:"sortOrder"`
	Active    bool               `bson:"active"        json:"active"`
	CreatedAt time.Time          `bson:"createdAt"     json:"createdAt"`
}

// CellImage is one image in the pool. TypeKey is the correct answer and must
// never be serialised to non-admin clients (B-01).
type CellImage struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"-"`
	TypeKey   string             `bson:"typeKey"       json:"-"`
	Path      string             `bson:"path"          json:"-"`
	Active    bool               `bson:"active"        json:"-"`
	CreatedAt time.Time          `bson:"createdAt"     json:"-"`
}

// CellTypeOption is the answer option shown to a user (snapshot in an attempt).
type CellTypeOption struct {
	Key   string `bson:"key"   json:"key"`
	Label string `bson:"label" json:"label"`
}

// CellTypeCounts is the image count per cell type used by the admin library.
type CellTypeCounts struct {
	Total  int
	Active int
}

// DefaultCellTypes are the WBC types known from the legacy data set. They seed
// the cell_types collection on migration/seed (never overwriting edits).
func DefaultCellTypes() []CellType {
	return []CellType{
		{Key: "neutrophil", Label: "Neutrophil", SortOrder: 10, Active: true},
		{Key: "lymphocyte", Label: "Lymphocyte", SortOrder: 20, Active: true},
		{Key: "monocyte", Label: "Monocyte", SortOrder: 30, Active: true},
		{Key: "eosinophil", Label: "Eosinophil", SortOrder: 40, Active: true},
		{Key: "basophil", Label: "Basophil", SortOrder: 50, Active: true},
		{Key: "erythroblast", Label: "Erythroblast", SortOrder: 60, Active: true},
	}
}
