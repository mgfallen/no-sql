package mongo

import (
	"context"
	"testing"

	"no-sql/cmd/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestMongoRepository_CreateAndGetUser(t *testing.T) {
	t.Parallel()
	repo, teardown := setupTestDB(t)
	defer teardown()
	ctx := context.Background()

	u := &domain.User{
		Username: "john_doe",
		FullName: "John Doe",
	}

	// Создание
	err := repo.CreateUser(ctx, u)
	assert.NoError(t, err)
	assert.NotEmpty(t, u.ID)

	// Получение по Username
	byName, err := repo.GetUserByUsername(ctx, "john_doe")
	assert.NoError(t, err)
	assert.Equal(t, "John Doe", byName.FullName)

	// Получение по ID
	byID, err := repo.GetUserByID(ctx, u.ID.Hex())
	assert.NoError(t, err)
	assert.Equal(t, "john_doe", byID.Username)
}

func TestMongoRepository_GetUser_NotFound(t *testing.T) {
	t.Parallel()
	repo, teardown := setupTestDB(t)
	defer teardown()
	ctx := context.Background()

	// Ошибка при невалидном формате Hex ID
	_, err := repo.GetUserByID(ctx, "invalid_hex_string")
	assert.ErrorIs(t, err, mongo.ErrNoDocuments)

	// Ошибка при отсутствии записи в БД
	_, err = repo.GetUserByUsername(ctx, "missing_user")
	assert.ErrorIs(t, err, mongo.ErrNoDocuments)
}

func TestMongoRepository_ListUsers_PaginationAndFilters(t *testing.T) {
	t.Parallel()
	repo, teardown := setupTestDB(t)
	defer teardown()
	ctx := context.Background()

	userAlex1 := &domain.User{Username: "alex1", FullName: "Alex Mercer"}
	userAlex2 := &domain.User{Username: "alex2", FullName: "Alex Vance"}
	userBob := &domain.User{Username: "bob", FullName: "Bob Smith"}

	_ = repo.CreateUser(ctx, userAlex1)
	_ = repo.CreateUser(ctx, userAlex2)
	_ = repo.CreateUser(ctx, userBob)

	t.Run("Filter by name with pagination", func(t *testing.T) {
		users, total, err := repo.ListUsers(ctx, "", "alex", 1, 0)
		assert.NoError(t, err)
		assert.Equal(t, int64(2), total) // Найдено всего совпадений
		assert.Len(t, users, 1)          // Вернулся только один элемент из-за лимита
		assert.Contains(t, users[0].FullName, "Alex")
	})

	t.Run("Filter by exact ID", func(t *testing.T) {
		users, total, err := repo.ListUsers(ctx, userAlex1.ID.Hex(), "", 10, 0)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, users, 1)
		assert.Equal(t, "alex1", users[0].Username)
	})

	t.Run("Filter by exact ID and matching Name", func(t *testing.T) {
		users, total, err := repo.ListUsers(ctx, userBob.ID.Hex(), "Bob", 10, 0)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, users, 1)
		assert.Equal(t, "bob", users[0].Username)
	})

	t.Run("Filter by exact ID and mismatching Name", func(t *testing.T) {
		users, total, err := repo.ListUsers(ctx, userBob.ID.Hex(), "Alex", 10, 0)
		assert.NoError(t, err)
		assert.Equal(t, int64(0), total)
		assert.Len(t, users, 0)
	})

	t.Run("Invalid Hex ID should handle gracefully", func(t *testing.T) {
		users, total, err := repo.ListUsers(ctx, "not_a_valid_hex", "", 10, 0)
		assert.NoError(t, err)
		assert.Equal(t, int64(0), total)
		assert.Len(t, users, 0)
	})
}
