package domain

import "go.mongodb.org/mongo-driver/bson/primitive"

type User struct {
	ID           primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	FullName     string             `json:"full_name" bson:"full_name"`
	Username     string             `json:"username" bson:"username"`
	PasswordHash string             `json:"-" bson:"password_hash"`
}

type Event struct {
	ID          primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Title       string             `json:"title" bson:"title"`
	Description string             `json:"description" bson:"description"`
	Location    struct {
		Address string `json:"address" bson:"address"`
	} `json:"location" bson:"location"`
	CreatedAt  string `json:"created_at" bson:"created_at"`
	CreatedBy  string `json:"created_by" bson:"created_by"` // здесь хранится hex-строка ID юзера
	StartedAt  string `json:"started_at" bson:"started_at"`
	FinishedAt string `json:"finished_at" bson:"finished_at"`
}
type Location struct {
	Address string `json:"address" bson:"address"`
}
