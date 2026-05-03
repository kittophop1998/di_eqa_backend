package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/di-eqa/backend/internal/models"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type HospitalHandler struct {
	Coll *mongo.Collection
}

func (h *HospitalHandler) List(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	q := strings.TrimSpace(c.Query("q"))
	filter := bson.M{}
	if q != "" {
		filter = bson.M{"$or": []bson.M{
			{"name": bson.M{"$regex": q, "$options": "i"}},
			{"code": bson.M{"$regex": q, "$options": "i"}},
		}}
	}

	cur, err := h.Coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}).SetLimit(200))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer cur.Close(ctx)

	var list []models.Hospital
	if err := cur.All(ctx, &list); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []models.Hospital{}
	}
	c.JSON(http.StatusOK, list)
}

func (h *HospitalHandler) GetByCode(c *gin.Context) {
	code := strings.ToUpper(strings.TrimSpace(c.Param("code")))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var hospital models.Hospital
	if err := h.Coll.FindOne(ctx, bson.M{"code": code}).Decode(&hospital); err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "hospital not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, hospital)
}
