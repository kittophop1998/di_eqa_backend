package entity

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CellCategory คือชนิดของเซลล์ที่ผู้ใช้ต้องจำแนก เช่น Neutrophil, Lymphocyte
type CellCategory struct {
	Key   string `bson:"key"   json:"key"`
	Label string `bson:"label" json:"label"`
	Color string `bson:"color" json:"color"`
}

// CellImage คือรูปเซลล์ 1 ใบในข้อสอบ
// ImageURL ไม่เก็บลง MongoDB — สร้างจาก SupabaseURL + bucket + Path ตอน request
type CellImage struct {
	ID          string             `bson:"id"                     json:"id"`
	AssetID     primitive.ObjectID `bson:"assetId,omitempty"      json:"assetId,omitempty"`
	Path        string             `bson:"path,omitempty"         json:"path,omitempty"`
	ImageURL    string             `bson:"-"                      json:"imageUrl,omitempty"`
	CorrectType string             `bson:"correctType"            json:"correctType,omitempty"`
}

// Quiz — Cell classification exam
type Quiz struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title       string             `bson:"title"         json:"title"`
	Description string             `bson:"description"   json:"description"`
	Category    string             `bson:"category"      json:"category"`
	Categories  []CellCategory     `bson:"categories"    json:"categories"`
	Cells       []CellImage        `bson:"cells"         json:"cells"`
	PassPercent int                `bson:"passPercent"   json:"passPercent"`
	DurationSec int                `bson:"durationSec"   json:"durationSec"`
	CreatedBy   primitive.ObjectID `bson:"createdBy"     json:"createdBy"`
	CreatedAt   time.Time          `bson:"createdAt"     json:"createdAt"`
}

// CellImageAsset เก็บ path ของรูปภาพบน Supabase Storage
type CellImageAsset struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	CellType  string             `bson:"cellType"      json:"cellType"`
	Filename  string             `bson:"filename"      json:"filename"`
	Path      string             `bson:"path"          json:"path"`
	CreatedAt time.Time          `bson:"createdAt"     json:"createdAt"`
}

// PublicCells คืน slice ที่ตัด CorrectType ออก (ImageURL ต้อง inject ก่อนเรียก)
func (q *Quiz) PublicCells() []CellImage {
	out := make([]CellImage, len(q.Cells))
	for i, c := range q.Cells {
		out[i] = CellImage{ID: c.ID, ImageURL: c.ImageURL}
	}
	return out
}
