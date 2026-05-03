package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	SessionPending = "pending"
	SessionRunning = "running"
	SessionEnded   = "ended"
)

type Session struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Code        string             `bson:"code" json:"code"`
	QuizID      primitive.ObjectID `bson:"quizId" json:"quizId"`
	HostID      primitive.ObjectID `bson:"hostId" json:"hostId"`
	HospitalID  primitive.ObjectID `bson:"hospitalId" json:"hospitalId"`
	Status      string             `bson:"status" json:"status"`
	StartedAt   *time.Time         `bson:"startedAt,omitempty" json:"startedAt,omitempty"`
	EndedAt     *time.Time         `bson:"endedAt,omitempty" json:"endedAt,omitempty"`
	CreatedAt   time.Time          `bson:"createdAt" json:"createdAt"`
}
