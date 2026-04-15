package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"no-sql/cmd/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"github.com/testcontainers/testcontainers-go/wait"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var testClient *mongo.Client

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Запуск реальной Mongo в Docker для тестов
	mongoContainer, err := mongodb.Run(ctx, "mongo:6.0",
		testcontainers.WithWaitStrategy(
			wait.ForLog("Waiting for connections").WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		fmt.Printf("Failed to start container: %v\n", err)
		os.Exit(1)
	}

	endpoint, err := mongoContainer.ConnectionString(ctx)
	if err != nil {
		fmt.Printf("Failed to get connection string: %v\n", err)
		os.Exit(1)
	}

	client, err := mongo.Connect(options.Client().ApplyURI(endpoint))
	if err != nil {
		fmt.Printf("Failed to connect to Mongo: %v\n", err)
		os.Exit(1)
	}

	testClient = client
	code := m.Run()

	_ = client.Disconnect(context.Background())
	_ = mongoContainer.Terminate(context.Background())
	os.Exit(code)
}

func setupTestDB(t *testing.T) (*mongo.Database, func()) {
	t.Helper()
	dbName := fmt.Sprintf("db_%d_%s", time.Now().UnixNano(), bson.NewObjectID().Hex())
	db := testClient.Database(dbName)
	return db, func() { _ = db.Drop(context.Background()) }
}

func TestMongoRepository_CreateUser(t *testing.T) {
	t.Parallel()
	db, teardown := setupTestDB(t)
	defer teardown()
	repo := NewMongoRepository(db)
	_ = repo.InitIndices(context.Background())

	t.Run("Success create", func(t *testing.T) {
		user := &domain.User{Username: "vperov", FullName: "Viktor Perov"}
		err := repo.CreateUser(context.Background(), user)
		assert.NoError(t, err)
		assert.False(t, user.ID.IsZero())
	})

	t.Run("Duplicate username", func(t *testing.T) {
		_ = repo.CreateUser(context.Background(), &domain.User{Username: "dup"})
		err := repo.CreateUser(context.Background(), &domain.User{Username: "dup"})
		assert.Error(t, err)
	})
}

func TestMongoRepository_GetEvents_Filters(t *testing.T) {
	t.Parallel()
	db, teardown := setupTestDB(t)
	defer teardown()
	repo := NewMongoRepository(db)
	ctx := context.Background()

	events := []interface{}{
		domain.Event{Title: "Go Workshop", Category: "tech", Price: 100, StartedAt: "20260401"},
		domain.Event{Title: "Java Meetup", Category: "tech", Price: 500, StartedAt: "20260410"},
		domain.Event{Title: "Party", Category: "fun", Price: 0, StartedAt: "20260420"},
	}
	_, _ = db.Collection("events").InsertMany(ctx, events)

	tests := []struct {
		name      string
		filters   map[string]interface{}
		wantCount int
	}{
		{
			name:      "Filter by category",
			filters:   map[string]interface{}{"category": "tech"},
			wantCount: 2,
		},
		{
			name:      "Filter by price range",
			filters:   map[string]interface{}{"price_from": uint(50), "price_to": uint(150)},
			wantCount: 1,
		},
		{
			name:      "Filter by date range",
			filters:   map[string]interface{}{"date_from": "20260405", "date_to": "20260415"},
			wantCount: 1,
		},
		{
			name:      "Combined filters",
			filters:   map[string]interface{}{"category": "tech", "price_from": uint(400)},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, total, err := repo.GetEvents(ctx, tt.filters, 10, 0)
			assert.NoError(t, err)
			assert.Len(t, res, tt.wantCount)
			assert.Equal(t, int64(tt.wantCount), total)
		})
	}
}

func TestMongoRepository_PatchEvent(t *testing.T) {
	t.Parallel()
	db, teardown := setupTestDB(t)
	defer teardown()
	repo := NewMongoRepository(db)
	ctx := context.Background()

	userID := "my-user-id"
	event := domain.Event{
		Title:     "Initial Event",
		CreatedBy: userID,
		Location:  domain.Location{City: "Moscow"},
	}
	res, _ := db.Collection("events").InsertOne(ctx, event)
	eventID := res.InsertedID.(bson.ObjectID).Hex()

	t.Run("Successful patch and unset city", func(t *testing.T) {
		updates := bson.M{
			"title":         "Updated Event",
			"location.city": "",
		}

		ok, err := repo.PatchEvent(ctx, eventID, userID, updates)
		assert.NoError(t, err)
		assert.True(t, ok)

		var updated domain.Event
		_ = db.Collection("events").FindOne(ctx, bson.M{"_id": res.InsertedID}).Decode(&updated)
		assert.Equal(t, "Updated Event", updated.Title)
		assert.Empty(t, updated.Location.City)
	})

	t.Run("Fail patch - wrong user", func(t *testing.T) {
		ok, err := repo.PatchEvent(ctx, eventID, "hacker-id", bson.M{"title": "Hacked"})
		assert.NoError(t, err)
		assert.False(t, ok, "MatchedCount should be 0 because of created_by check")
	})

	t.Run("Fail patch - invalid ID", func(t *testing.T) {
		ok, err := repo.PatchEvent(ctx, "invalid-hex", userID, bson.M{"title": "Fail"})
		assert.Error(t, err)
		assert.False(t, ok)
	})
}
