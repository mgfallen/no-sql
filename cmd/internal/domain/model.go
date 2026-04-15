package domain

import "go.mongodb.org/mongo-driver/v2/bson"

type Location struct {
	Address string `json:"address" bson:"address"`
	City    string `json:"city,omitempty" bson:"city,omitempty"`
}

type User struct {
	ID           bson.ObjectID `bson:"_id,omitempty" json:"id"`
	FullName     string        `bson:"full_name" json:"full_name"`
	Username     string        `bson:"username" json:"username"`
	PasswordHash string        `bson:"password_hash" json:"-"`
	Password     string        `bson:"-" json:"password,omitempty"`
}

type Event struct {
	ID          bson.ObjectID `json:"id" bson:"_id,omitempty"`
	Title       string        `json:"title" bson:"title"`
	Description string        `json:"description" bson:"description"`
	Category    string        `json:"category" bson:"category"`
	Price       uint          `json:"price" bson:"price"`
	Location    Location      `json:"location" bson:"location"`
	CreatedAt   string        `json:"created_at" bson:"created_at"`
	CreatedBy   string        `json:"created_by" bson:"created_by"`
	StartedAt   string        `json:"started_at" bson:"started_at"`
	FinishedAt  string        `json:"finished_at" bson:"finished_at"`
}
