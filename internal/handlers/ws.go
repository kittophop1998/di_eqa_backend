package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/di-eqa/backend/internal/models"
	"github.com/di-eqa/backend/internal/ws"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type WSHandler struct {
	Hub          *ws.Hub
	UsersColl    *mongo.Collection
	HospitalColl *mongo.Collection
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func (h *WSHandler) Handle(c *gin.Context) {
	uid, _ := c.Get("userId")
	userID, err := primitive.ObjectIDFromHex(uid.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	var user models.User
	if err := h.UsersColl.FindOne(ctx, bson.M{"_id": userID}).Decode(&user); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	var hospital models.Hospital
	_ = h.HospitalColl.FindOne(ctx, bson.M{"_id": user.HospitalID}).Decode(&hospital)

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}

	client := ws.NewClient(h.Hub, conn, user.ID.Hex(), user.Username, user.FullName, hospital.Name)
	h.Hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}
