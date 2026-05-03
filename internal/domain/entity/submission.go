package entity

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CellAnswer แทน 1 รูปเซลล์ที่ผู้ใช้ตอบ
type CellAnswer struct {
	CellID       string `bson:"cellId"       json:"cellId"`
	ImageURL     string `bson:"imageUrl"     json:"imageUrl"`
	AssignedType string `bson:"assignedType" json:"assignedType"`
	CorrectType  string `bson:"correctType"  json:"correctType"`
	IsCorrect    bool   `bson:"isCorrect"    json:"isCorrect"`
}

// CategoryStat สรุปต่อชนิดเซลล์
type CategoryStat struct {
	Key       string `bson:"key"       json:"key"`
	Label     string `bson:"label"     json:"label"`
	Color     string `bson:"color"     json:"color"`
	UserCount int    `bson:"userCount" json:"userCount"`
	TrueCount int    `bson:"trueCount" json:"trueCount"`
	Correct   int    `bson:"correct"   json:"correct"`
}

// Submission แทนการส่งคำตอบ 1 ครั้ง
type Submission struct {
	ID            primitive.ObjectID  `bson:"_id,omitempty"          json:"id"`
	QuizID        primitive.ObjectID  `bson:"quizId"                 json:"quizId"`
	QuizTitle     string              `bson:"quizTitle"              json:"quizTitle"`
	SessionID     *primitive.ObjectID `bson:"sessionId,omitempty"    json:"sessionId,omitempty"`
	UserID        primitive.ObjectID  `bson:"userId"                 json:"userId"`
	HospitalID    primitive.ObjectID  `bson:"hospitalId"             json:"hospitalId"`
	Username      string              `bson:"username"               json:"username"`
	FullName      string              `bson:"fullName"               json:"fullName"`
	HospitalName  string              `bson:"hospitalName"           json:"hospitalName"`
	Score         int                 `bson:"score"                  json:"score"`
	Total         int                 `bson:"total"                  json:"total"`
	Correct       int                 `bson:"correct"                json:"correct"`
	Percent       int                 `bson:"percent"                json:"percent"`
	PassPercent   int                 `bson:"passPercent"            json:"passPercent"`
	Passed        bool                `bson:"passed"                 json:"passed"`
	CertificateID string              `bson:"certificateId,omitempty" json:"certificateId,omitempty"`
	Categories    []CategoryStat      `bson:"categories"             json:"categories"`
	Answers       []CellAnswer        `bson:"answers"                json:"answers"`
	DurationSec   int                 `bson:"durationSec"            json:"durationSec"`
	SubmittedAt   time.Time           `bson:"submittedAt"            json:"submittedAt"`
}
