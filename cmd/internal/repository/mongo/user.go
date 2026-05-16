package mongo

import (
	"context"

	"no-sql/cmd/internal/domain"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func (r *Repository) CreateUser(ctx context.Context, user *domain.User) error {
	res, err := r.db.Collection("users").InsertOne(ctx, user)
	if err != nil {
		return err
	}
	if oid, ok := res.InsertedID.(bson.ObjectID); ok {
		user.ID = oid
	}
	return nil
}

func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user domain.User
	err := r.db.Collection("users").FindOne(ctx, bson.M{"username": username}).Decode(&user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, mongo.ErrNoDocuments
	}
	var user domain.User
	err = r.db.Collection("users").FindOne(ctx, bson.M{"_id": oid}).Decode(&user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// ListUsers теперь поддерживает фильтрацию как по имени (name), так и по точному совпадению ID (id)
func (r *Repository) ListUsers(ctx context.Context, id string, name string, limit, offset int64) ([]domain.User, int64, error) {
	filter := bson.M{}

	if id != "" {
		oid, err := bson.ObjectIDFromHex(id)
		if err != nil {
			return []domain.User{}, 0, nil
		}
		filter["_id"] = oid
	}

	if name != "" {
		filter["full_name"] = bson.M{"$regex": name, "$options": "i"}
	}

	count, err := r.db.Collection("users").CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().SetLimit(limit).SetSkip(offset)
	cursor, err := r.db.Collection("users").Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	users := make([]domain.User, 0)
	if err = cursor.All(ctx, &users); err != nil {
		return nil, 0, err
	}

	return users, count, nil
}
