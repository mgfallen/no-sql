package mongo

import (
	"context"
	"strings"
	"testing"

	"no-sql/cmd/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type indexInfo struct {
	Name   string `bson:"name"`
	Key    bson.M `bson:"key"`
	Unique bool   `bson:"unique,omitempty"`
}

func TestMongoRepository_InitIndices(t *testing.T) {
	t.Parallel()
	repo, teardown := setupTestDB(t)
	defer teardown()
	ctx := context.Background()

	t.Run("User username unique constraint", func(t *testing.T) {
		err := repo.InitIndices(ctx)
		if err != nil {
			// Игнорируется ошибка шардирования на одиночной ноде
			require.True(t, strings.Contains(err.Error(), "CommandNotFound") || strings.Contains(err.Error(), "no such command"), "Unexpected error: %v", err)
		}

		user1 := &domain.User{Username: "clone_user", FullName: "First"}
		user2 := &domain.User{Username: "clone_user", FullName: "Second"}

		err = repo.CreateUser(ctx, user1)
		assert.NoError(t, err)

		err = repo.CreateUser(ctx, user2)
		assert.Error(t, err)

		var wExc mongo.WriteException
		if assert.ErrorAs(t, err, &wExc) {
			isDup := false
			for _, we := range wExc.WriteErrors {
				if we.Code == 11000 {
					isDup = true
				}
			}
			assert.True(t, isDup, "Ожидалась ошибка дублирования ключа (11000) для username")
		}
	})

	t.Run("Events collection index verification", func(t *testing.T) {
		err := repo.InitIndices(ctx)
		if err != nil {
			// Игнорируется ошибка шардирования на одиночной ноде
			require.True(t, strings.Contains(err.Error(), "CommandNotFound") || strings.Contains(err.Error(), "no such command"), "Unexpected error: %v", err)
		}

		cursor, err := repo.db.Collection("events").Indexes().List(ctx)
		require.NoError(t, err)
		defer cursor.Close(ctx)

		var indexes []indexInfo
		err = cursor.All(ctx, &indexes)
		require.NoError(t, err)

		hasCreatedByHashed := false

		for _, idx := range indexes {
			if val, ok := idx.Key["created_by"]; ok {
				if valStr, ok := val.(string); ok && valStr == "hashed" {
					hasCreatedByHashed = true
					assert.False(t, idx.Unique, "Хэшированный индекс не должен быть уникальным")
				}
			}
		}

		assert.True(t, hasCreatedByHashed, "Хэшированный индекс на поле 'created_by' отсутствует в коллекции events")
	})
}
