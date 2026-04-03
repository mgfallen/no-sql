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

	// Обновленные импорты для v2
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var testClient *mongo.Client

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

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

	client, err := mongo.Connect(options.Client().ApplyURI(endpoint).SetMaxPoolSize(50))
	if err != nil {
		fmt.Printf("Failed to connect to Mongo: %v\n", err)
		os.Exit(1)
	}

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := client.Ping(pingCtx, nil); err != nil {
		pingCancel()
		fmt.Printf("Failed to ping Mongo: %v\n", err)
		os.Exit(1)
	}
	pingCancel()

	testClient = client
	code := m.Run()

	_ = client.Disconnect(context.Background())
	_ = mongoContainer.Terminate(context.Background())
	os.Exit(code)
}

func setupTestDB(t *testing.T) (*mongo.Database, func()) {
	t.Helper()
	// Используем primitive из v2
	dbName := fmt.Sprintf("db_%d_%s", time.Now().UnixNano(), bson.NewObjectID().Hex())
	db := testClient.Database(dbName)
	teardown := func() {
		_ = db.Drop(context.Background())
	}
	return db, teardown
}

func TestMongoRepository_UserOps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		username      string
		prepare       func(ctx context.Context, repo *MongoRepository)
		expectedError error
		checkResult   func(t *testing.T, user *domain.User)
	}{
		{
			name:     "Success: create and get user",
			username: "ivan_ivanov",
			prepare: func(ctx context.Context, repo *MongoRepository) {
				_ = repo.CreateUser(ctx, &domain.User{Username: "ivan_ivanov", FullName: "Ivan"})
			},
			expectedError: nil,
			checkResult: func(t *testing.T, user *domain.User) {
				assert.NotNil(t, user)
				assert.Equal(t, "Ivan", user.FullName)
			},
		},
		{
			name:          "Failure: user not found",
			username:      "ghost_user",
			prepare:       func(ctx context.Context, repo *MongoRepository) {},
			expectedError: mongo.ErrNoDocuments,
			checkResult:   func(t *testing.T, user *domain.User) { assert.Nil(t, user) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, teardown := setupTestDB(t)
			defer teardown()
			repo := NewMongoRepository(db)
			ctx := context.Background()

			tt.prepare(ctx, repo)

			user, err := repo.GetUserByUsername(ctx, tt.username)

			if tt.expectedError != nil {
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
			tt.checkResult(t, user)
		})
	}
}

func TestMongoRepository_CreateEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		event         *domain.Event
		prepare       func(ctx context.Context, db *mongo.Database)
		expectedError bool
	}{
		{
			name: "Success: unique event",
			event: &domain.Event{
				Title:     "Unique Party",
				CreatedBy: bson.NewObjectID().Hex(),
			},
			prepare:       func(ctx context.Context, db *mongo.Database) {},
			expectedError: false,
		},
		{
			name: "Failure: duplicate title",
			event: &domain.Event{
				Title: "Repeat",
			},
			prepare: func(ctx context.Context, db *mongo.Database) {
				// Используем domain.Event{} напрямую
				_, _ = db.Collection("events").InsertOne(ctx, domain.Event{Title: "Repeat"})
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, teardown := setupTestDB(t)
			defer teardown()
			repo := NewMongoRepository(db)
			ctx := context.Background()

			_ = repo.InitIndices(ctx)

			tt.prepare(ctx, db)

			id, err := repo.CreateEvent(ctx, tt.event)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Empty(t, id)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, id)
			}
		})
	}
}

func TestMongoRepository_ListEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		query     string
		prepare   func(ctx context.Context, db *mongo.Database)
		wantCount int
		wantTotal int64
	}{
		{
			name:  "Find exact",
			query: "Go Workshop",
			prepare: func(ctx context.Context, db *mongo.Database) {
				events := []interface{}{
					domain.Event{Title: "Go Workshop"},
					domain.Event{Title: "Redis Talk"},
				}
				_, _ = db.Collection("events").InsertMany(ctx, events)
			},
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name:  "Find none",
			query: "Java",
			prepare: func(ctx context.Context, db *mongo.Database) {
				_, _ = db.Collection("events").InsertOne(ctx, domain.Event{Title: "C++"})
			},
			wantCount: 0,
			wantTotal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, teardown := setupTestDB(t)
			defer teardown()
			repo := NewMongoRepository(db)
			ctx := context.Background()

			tt.prepare(ctx, db)

			res, total, err := repo.GetEvents(ctx, tt.query, 10, 0)

			assert.NoError(t, err)
			assert.Len(t, res, tt.wantCount)
			assert.Equal(t, tt.wantTotal, total)
		})
	}
}
