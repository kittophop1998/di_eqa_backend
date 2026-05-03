package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CellCategory คือชนิดของเซลล์ที่ผู้ใช้ต้องจำแนก เช่น Neutrophil, Lymphocyte
// ใช้ทั้งฝั่ง quiz (label ฝั่งขวา) และ ฝั่ง submission (สรุปจำนวนต่อชนิด)
type CellCategory struct {
	Key   string `bson:"key" json:"key"`     // canonical key เช่น "neutrophil"
	Label string `bson:"label" json:"label"` // ป้ายแสดงผล เช่น "Neutrophil"
	Color string `bson:"color" json:"color"` // hex color สำหรับ chip ฝั่ง UI
}

// CellImage คือรูปเซลล์ 1 ใบในข้อสอบ — ผู้ใช้ต้องบอกว่าเซลล์นี้เป็นชนิดอะไร
// AssetID อ้างอิง _id ใน collection cell_image_assets
// Path คือ relative path บน Supabase bucket เช่น "neutrophil/BNE_100878.jpg"
// ImageURL ไม่เก็บลง MongoDB — สร้างจาก SupabaseURL + bucket + Path ตอน request เข้ามา
type CellImage struct {
	ID          string             `bson:"id" json:"id"`
	AssetID     primitive.ObjectID `bson:"assetId,omitempty" json:"assetId,omitempty"`
	Path        string             `bson:"path,omitempty" json:"path,omitempty"` // relative path on Supabase
	ImageURL    string             `bson:"-" json:"imageUrl,omitempty"`          // computed at query time, never stored
	CorrectType string             `bson:"correctType" json:"correctType,omitempty"`
}

// Quiz ในเวอร์ชันนี้เปลี่ยนจาก "Multiple choice" → "Cell classification"
// ไม่มี Questions/Choices แล้ว แต่มี Cells (รูปที่ต้องจำแนก) + Categories (ปลายทาง)
type Quiz struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title       string             `bson:"title" json:"title"`
	Description string             `bson:"description" json:"description"`
	Category    string             `bson:"category" json:"category"`
	Categories  []CellCategory     `bson:"categories" json:"categories"`
	Cells       []CellImage        `bson:"cells" json:"cells"`
	PassPercent int                `bson:"passPercent" json:"passPercent"`
	DurationSec int                `bson:"durationSec" json:"durationSec"`
	CreatedBy   primitive.ObjectID `bson:"createdBy" json:"createdBy"`
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`
}

// CellImageAsset เก็บ path ของรูปภาพจริงที่อัปโหลดไว้บน Supabase Storage
// path จะอยู่ในรูป "{cellType}/{filename}" เช่น "neutrophil/BNE_100878.jpg"
// ใช้ต่อกับ Supabase public URL เพื่อแสดงรูปตอนทำข้อสอบ
type CellImageAsset struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	CellType  string             `bson:"cellType" json:"cellType"` // ชนิดเซลล์ เช่น "neutrophil"
	Filename  string             `bson:"filename" json:"filename"` // ชื่อไฟล์ เช่น "BNE_100878.jpg"
	Path      string             `bson:"path" json:"path"`         // path บน Supabase: "{cellType}/{filename}"
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
}

// PublicCells คืน slice ที่ตัด CorrectType ออก ใช้ส่งให้ฝั่ง client
// เพื่อกันการเฉลยเซลล์รั่วผ่าน DevTools
// ImageURL ต้องถูก inject ไว้ก่อน (โดย handler) ก่อนเรียก method นี้
func (q *Quiz) PublicCells() []CellImage {
	out := make([]CellImage, len(q.Cells))
	for i, c := range q.Cells {
		out[i] = CellImage{ID: c.ID, ImageURL: c.ImageURL}
	}
	return out
}
