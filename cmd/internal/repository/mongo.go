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

func (r *MongoRepository) GetEvents(ctx context.Context, filters map[string]interface{}, limit, offset int64) ([]domain.Event, int64, error) {
	mongoFilter := bson.M{}

	// Базовые фильтры из ЛР3
	if t, ok := filters["title"].(string); ok && t != "" {
		mongoFilter["title"] = bson.M{"$regex": t, "$options": "i"}
	}
	if id, ok := filters["id"].(string); ok && id != "" {
		oid, _ := bson.ObjectIDFromHex(id)
		mongoFilter["_id"] = oid
	}

	// Новые фильтры ЛР4
	if cat, ok := filters["category"].(string); ok && cat != "" {
		mongoFilter["category"] = cat
	}
	if city, ok := filters["city"].(string); ok && city != "" {
		mongoFilter["location.city"] = city
	}
	if user, ok := filters["user"].(string); ok && user != "" {
		mongoFilter["created_by_name"] = user // Предполагаем, что имя создателя денормализовано или ищем по ID
	}

	// Фильтр по цене
	priceFilter := bson.M{}
	if pf, ok := filters["price_from"].(uint); ok {
		priceFilter["$gte"] = pf
	}
	if pt, ok := filters["price_to"].(uint); ok {
		priceFilter["$lte"] = pt
	}
	if len(priceFilter) > 0 {
		mongoFilter["price"] = priceFilter
	}

	// Фильтр по датам (формат YYYYMMDD)
	dateFilter := bson.M{}
	if df, ok := filters["date_from"].(string); ok && df != "" {
		dateFilter["$gte"] = df
	}
	if dt, ok := filters["date_to"].(string); ok && dt != "" {
		dateFilter["$lte"] = dt
	}
	if len(dateFilter) > 0 {
		mongoFilter["started_at"] = dateFilter
	}

	count, err := r.db.Collection("events").CountDocuments(ctx, mongoFilter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().SetLimit(limit).SetSkip(offset)
	cursor, err := r.db.Collection("events").Find(ctx, mongoFilter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var events []domain.Event
	if err := cursor.All(ctx, &events); err != nil {
		return nil, 0, err
	}

	return events, count, nil
}

func (r *MongoRepository) PatchEvent(ctx context.Context, eventID string, userID string, updates bson.M) (bool, error) {
	oid, err := bson.ObjectIDFromHex(eventID)
	if err != nil {
		return false, err
	}

	filter := bson.M{"_id": oid, "created_by": userID}

	res, err := r.db.Collection("events").UpdateOne(ctx, filter, bson.M{"$set": updates})
	if err != nil {
		return false, err
	}

	if city, ok := updates["location.city"]; ok && city == "" {
		_, _ = r.db.Collection("events").UpdateOne(ctx, filter, bson.M{"$unset": bson.M{"location.city": ""}})
	}

	return res.MatchedCount > 0, nil
}
