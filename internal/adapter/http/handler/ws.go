package handler

import (
	"context"
	"log"
	"net/http"
	"time"

	utils "github.com/di-eqa/backend/internal/adapter/http/response"
	"github.com/di-eqa/backend/internal/adapter/ws"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// userLookup is a minimal interface for resolving a user+hospital from an ID.
// This keeps WSHandler decoupled from concrete repositories.
type userLookup interface {
	FindByID(ctx context.Context, id primitive.ObjectID) (username, fullName, hospitalName string, err error)
}

// WSHandler is the HTTP driving adapter for WebSocket connections.
type WSHandler struct {
	hub        *ws.Hub
	userLookup userLookup
}

func NewWSHandler(hub *ws.Hub, ul userLookup) *WSHandler {
	return &WSHandler{hub: hub, userLookup: ul}
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
		utils.ErrUnauthorized(c, "invalid user")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	username, fullName, hospitalName, err := h.userLookup.FindByID(ctx, userID)
	if err != nil {
		utils.ErrUnauthorized(c, "user not found")
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}

	client := ws.NewClient(h.hub, conn, userID.Hex(), username, fullName, hospitalName)
	h.hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}
