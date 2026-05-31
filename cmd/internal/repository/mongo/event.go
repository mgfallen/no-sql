package mongo

import (
	"context"
	"strconv"
	"time"

	"no-sql/cmd/internal/domain"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func (r *Repository) CreateEvent(ctx context.Context, event *domain.Event) (string, error) {
	event.CreatedAt = time.Now().Format(time.RFC3339)
	if event.Category == "" {
		event.Category = "other"
	}

	res, err := r.collection.InsertOne(ctx, event)
	if err != nil {
		return "", err
	}

	if oid, ok := res.InsertedID.(bson.ObjectID); ok {
		return oid.Hex(), nil
	}
	return "", nil
}

func (r *Repository) GetEventByID(ctx context.Context, id string) (*domain.Event, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, mongo.ErrNoDocuments
	}

	var event domain.Event
	err = r.collection.FindOne(ctx, bson.M{"_id": oid}).Decode(&event)
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (r *Repository) UpdateEvent(ctx context.Context, id string, organizerID string, category string, price *uint64, city *string) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return mongo.ErrNoDocuments
	}

	update := bson.M{}
	setFields := bson.M{}
	unsetFields := bson.M{}

	if category != "" {
		setFields["category"] = category
	}
	if price != nil {
		setFields["price"] = *price
	}
	if city != nil {
		if *city == "" {
			unsetFields["location.city"] = ""
		} else {
			setFields["location.city"] = *city
		}
	}

	if len(setFields) > 0 {
		update["$set"] = setFields
	}
	if len(unsetFields) > 0 {
		update["$unset"] = unsetFields
	}

	if len(update) == 0 {
		return nil
	}

	filterString := bson.M{
		"_id":        oid,
		"created_by": organizerID,
	}
	res, err := r.collection.UpdateOne(ctx, filterString, update)
	if err != nil {
		return err
	}

	if res.MatchedCount > 0 {
		return nil
	}

	if userOid, errParse := bson.ObjectIDFromHex(organizerID); errParse == nil {
		filterObjectID := bson.M{
			"_id":        oid,
			"created_by": userOid,
		}
		res, err = r.collection.UpdateOne(ctx, filterObjectID, update)
		if err != nil {
			return err
		}
		if res.MatchedCount > 0 {
			return nil
		}
	}

	return mongo.ErrNoDocuments
}

func (r *Repository) QueryEvents(ctx context.Context, filters map[string]string, limit, offset int64) ([]domain.Event, int64, error) {
	filter := bson.M{}

	if id, ok := filters["id"]; ok && id != "" {
		if objID, err := bson.ObjectIDFromHex(id); err == nil {
			filter["_id"] = objID
		}
	}

	if title, ok := filters["title"]; ok && title != "" {
		filter["title"] = title
	}

	if category, ok := filters["category"]; ok && category != "" {
		filter["category"] = category
	}
	if city, ok := filters["city"]; ok && city != "" {
		filter["location.city"] = city
	}

	if userStr, ok := filters["user"]; ok && userStr != "" {
		var u bson.M
		err := r.db.Collection("users").FindOne(ctx, bson.M{"username": userStr}).Decode(&u)
		if err == nil {
			allowedCreators := []interface{}{userStr}
			if id, exists := u["_id"]; exists {
				allowedCreators = append(allowedCreators, id)
				if oid, ok := id.(bson.ObjectID); ok {
					allowedCreators = append(allowedCreators, oid.Hex())
				}
			}
			filter["created_by"] = bson.M{"$in": allowedCreators}
		} else {
			filter["created_by"] = userStr
		}
	}

	// 5. Фильтр по цене
	priceFilter := bson.M{}
	if priceFromStr, ok := filters["price_from"]; ok && priceFromStr != "" {
		if pf, err := strconv.ParseUint(priceFromStr, 10, 64); err == nil {
			priceFilter["$gte"] = pf
		}
	}
	if priceToStr, ok := filters["price_to"]; ok && priceToStr != "" {
		if pt, err := strconv.ParseUint(priceToStr, 10, 64); err == nil {
			priceFilter["$lte"] = pt
		}
	}
	if len(priceFilter) > 0 {
		filter["price"] = priceFilter
	}

	// 6. Фильтр по дате
	dateFromStr := filters["date_from"]
	if dateFromStr == "" {
		dateFromStr = filters["started_date_from"]
	}
	dateToStr := filters["date_to"]
	if dateToStr == "" {
		dateToStr = filters["started_date_to"]
	}

	dateFilter := bson.M{}
	if dateFromStr != "" {
		dateFilter["$gte"] = dateFromStr
	}
	if dateToStr != "" {
		dateFilter["$lte"] = dateToStr
	}
	if len(dateFilter) > 0 {
		filter["started_at"] = dateFilter
	}

	opts := options.Find().SetLimit(limit).SetSkip(offset)
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var events []domain.Event
	if err = cursor.All(ctx, &events); err != nil {
		return nil, 0, err
	}
	if events == nil {
		events = []domain.Event{}
	}

	count, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return events, count, nil
}
