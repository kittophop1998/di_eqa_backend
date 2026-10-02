package db

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Mongo wraps the MongoDB client and selected database.
type Mongo struct {
	Client *mongo.Client
	DB     *mongo.Database
}

func Connect(uri, dbName string) (*Mongo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	log.Printf("✅ MongoDB connected: %s", uri)
	return &Mongo{Client: client, DB: client.Database(dbName)}, nil
}

func (m *Mongo) Close() {
	if m == nil || m.Client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = m.Client.Disconnect(ctx)
}
