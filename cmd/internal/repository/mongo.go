package repository

import (
	"context"
	"errors"

	"no-sql/cmd/internal/domain" // проверь правильность пути

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoRepository - репозиторий с монгой
type MongoRepository struct {
	db *mongo.Database
}

// NewMongoRepository - конструктор
func NewMongoRepository(db *mongo.Database) *MongoRepository {
	return &MongoRepository{db: db}
}

// InitIndices - инициализация индексов
func (r *MongoRepository) InitIndices(ctx context.Context) error {
	_, err := r.db.Collection("users").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "username", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return err
	}

	eventIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "title", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "title", Value: 1}, {Key: "created_by", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_by", Value: 1}},
		},
	}

	_, err = r.db.Collection("events").Indexes().CreateMany(ctx, eventIndexes)
	return err
}

// CreateUser - создание пользователя
func (r *MongoRepository) CreateUser(ctx context.Context, user *domain.User) error {
	_, err := r.db.Collection("users").InsertOne(ctx, user)
	return err
}

// GetUserByUsername - поиск пользователя
func (r *MongoRepository) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user domain.User
	err := r.db.Collection("users").FindOne(ctx, bson.M{"username": username}).Decode(&user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// CreateEvent - создание события
func (r *MongoRepository) CreateEvent(ctx context.Context, event *domain.Event) (string, error) {
	res, err := r.db.Collection("events").InsertOne(ctx, event)
	if err != nil {
		return "", err
	}
	// Кастуем InsertedID к ObjectID и возвращаем Hex-строку
	if oid, ok := res.InsertedID.(primitive.ObjectID); ok {
		return oid.Hex(), nil
	}
	return "", errors.New("failed to convert inserted id to hex")
}

// ListEvents - поиск с фильтрацией
func (r *MongoRepository) ListEvents(ctx context.Context, title string, limit, offset int64) ([]domain.Event, error) {
	filter := bson.M{}
	if title != "" {
		filter["title"] = bson.M{"$regex": title, "$options": "i"}
	}

	opts := options.Find().SetLimit(limit).SetSkip(offset)
	cursor, err := r.db.Collection("events").Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	events := make([]domain.Event, 0)
	if err = cursor.All(ctx, &events); err != nil {
		return nil, err
	}

	return events, nil
}
