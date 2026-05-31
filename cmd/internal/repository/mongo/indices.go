package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func (r *Repository) InitIndices(ctx context.Context) error {
	dbName := r.db.Name()

	usersCollection := r.db.Collection("users")
	_, err := usersCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "username", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("failed to create unique index on users.username: %w", err)
	}

	eventsCollection := r.db.Collection("events")

	_, err = eventsCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "title", Value: 1}},
	})
	if err != nil {
		return fmt.Errorf("failed to create index on events.title: %w", err)
	}

	_, err = eventsCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "created_by", Value: "hashed"}},
	})
	if err != nil {
		return fmt.Errorf("failed to create index on events.created_by: %w", err)
	}

	adminDB := r.client.Database("admin")
	_ = adminDB.RunCommand(ctx, bson.D{{Key: "enableSharding", Value: dbName}})

	fullCollectionName := fmt.Sprintf("%s.events", dbName)
	shardCommand := bson.D{
		{Key: "shardCollection", Value: fullCollectionName},
		{Key: "key", Value: bson.D{{Key: "created_by", Value: "hashed"}}},
	}

	err = adminDB.RunCommand(ctx, shardCommand).Err()
	if err != nil {
		return fmt.Errorf("failed to shard collection %s: %w", fullCollectionName, err)
	}

	return nil
}
