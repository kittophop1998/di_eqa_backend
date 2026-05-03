package main

import "go.mongodb.org/mongo-driver/mongo"

// initCollections resolves every MongoDB collection the application needs
// from the connected database and groups them into a single struct.
func initCollections(database *mongo.Database) *collections {
	return &collections{
		hospitals:   database.Collection("hospitals"),
		users:       database.Collection("users"),
		quizzes:     database.Collection("quizzes"),
		submissions: database.Collection("submissions"),
		sessions:    database.Collection("sessions"),
	}
}
