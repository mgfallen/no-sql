package repository

import (
	"context"
	"time"

	"no-sql/cmd/internal/domain"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type MongoRepository struct {
	db *mongo.Database
}

func NewMongoRepository(db *mongo.Database) *MongoRepository {
	return &MongoRepository{db: db}
}

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

func (r *MongoRepository) CreateUser(ctx context.Context, user *domain.User) error {
	res, err := r.db.Collection("users").InsertOne(ctx, user)
	if err != nil {
		return err
	}

	if oid, ok := res.InsertedID.(bson.ObjectID); ok {
		user.ID = oid
	}
	return nil
}

func (r *MongoRepository) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user domain.User
	err := r.db.Collection("users").FindOne(ctx, bson.M{"username": username}).Decode(&user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *MongoRepository) CreateEvent(ctx context.Context, event *domain.Event) (string, error) {
	event.CreatedAt = time.Now().Format(time.RFC3339)
	res, err := r.db.Collection("events").InsertOne(ctx, event)
	if err != nil {
		return "", err
	}

	if oid, ok := res.InsertedID.(bson.ObjectID); ok {
		return oid.Hex(), nil
	}

	return "", nil
}

func (r *MongoRepository) GetEvents(ctx context.Context, title string, limit, offset int64) ([]domain.Event, int64, error) {
	filter := bson.M{}
	if title != "" {
		filter["title"] = bson.M{"$regex": title, "$options": "i"}
	}

	count, err := r.db.Collection("events").CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().SetLimit(limit).SetSkip(offset)
	cursor, err := r.db.Collection("events").Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	events := make([]domain.Event, 0)
	if err = cursor.All(ctx, &events); err != nil {
		return nil, 0, err
	}

	return events, count, nil
}
