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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var testClient *mongo.Client

func TestMain(m *testing.M) {
	os.Setenv("TESTCONTAINERS_DOCKER_ROOTLESS", "false")

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

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(endpoint).SetMaxPoolSize(50))
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
	dbName := fmt.Sprintf("db_%d_%s", time.Now().UnixNano(), primitive.NewObjectID().Hex())
	db := testClient.Database(dbName)
	teardown := func() {
		go func() {
			_ = db.Drop(context.Background())
		}()
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
			t.Parallel()
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
				CreatedBy: primitive.NewObjectID().Hex(),
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
				_, _ = db.Collection("events").InsertOne(ctx, domain.Event{Title: "Repeat"})
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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
	}{
		{
			name:  "Find exact",
			query: "Go Workshop",
			prepare: func(ctx context.Context, db *mongo.Database) {
				_, _ = db.Collection("events").InsertMany(ctx, []interface{}{
					domain.Event{Title: "Go Workshop"},
					domain.Event{Title: "Redis Talk"},
				})
			},
			wantCount: 1,
		},
		{
			name:  "Find none",
			query: "Java",
			prepare: func(ctx context.Context, db *mongo.Database) {
				_, _ = db.Collection("events").InsertOne(ctx, domain.Event{Title: "C++"})
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db, teardown := setupTestDB(t)
			defer teardown()
			repo := NewMongoRepository(db)
			ctx := context.Background()

			tt.prepare(ctx, db)

			res, err := repo.ListEvents(ctx, tt.query, 10, 0)

			assert.NoError(t, err)
			assert.Len(t, res, tt.wantCount)
		})
	}
}
