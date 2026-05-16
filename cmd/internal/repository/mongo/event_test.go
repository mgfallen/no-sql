package mongo

import (
	"context"
	"testing"

	"no-sql/cmd/internal/domain"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestMongoRepository_CreateEvent_Defaults(t *testing.T) {
	t.Parallel()
	repo, teardown := setupTestDB(t)
	defer teardown()
	ctx := context.Background()

	ev := &domain.Event{
		Title:     "Concert",
		CreatedBy: "org_123",
	}

	id, err := repo.CreateEvent(ctx, ev)
	assert.NoError(t, err)
	assert.NotEmpty(t, id)

	// Проверяем, что проставились дефолтная категория и дата создания
	saved, err := repo.GetEventByID(ctx, id)
	assert.NoError(t, err)
	assert.Equal(t, "other", saved.Category)
	assert.NotEmpty(t, saved.CreatedAt)
}

func TestMongoRepository_UpdateEvent_PATCH(t *testing.T) {
	t.Parallel()
	repo, teardown := setupTestDB(t)
	defer teardown()
	ctx := context.Background()

	orgID := "65e9c0b1a2b3c4d5e6f7a8b9"
	city := "Moscow"
	ev := &domain.Event{
		Title:     "Go Workshop",
		Category:  "education",
		CreatedBy: orgID,
		Location:  domain.Location{City: &city},
	}
	id, err := repo.CreateEvent(ctx, ev)
	assert.NoError(t, err)

	newPrice := uint64(500)
	err = repo.UpdateEvent(ctx, id, orgID, "", &newPrice, nil)
	assert.NoError(t, err)

	emptyCity := ""
	err = repo.UpdateEvent(ctx, id, orgID, "tech", nil, &emptyCity)
	assert.NoError(t, err)

	updated, err := repo.GetEventByID(ctx, id)
	assert.NoError(t, err)
	assert.Equal(t, uint64(500), updated.Price)
	assert.Equal(t, "tech", updated.Category)
	assert.Nil(t, updated.Location.City)

	err = repo.UpdateEvent(ctx, id, "65e9c0b1a2b3c4d5e6f7a8b0", "hacked", nil, nil)
	assert.ErrorIs(t, err, mongo.ErrNoDocuments)
}

func TestMongoRepository_QueryEvents_Filters(t *testing.T) {
	t.Parallel()
	repo, teardown := setupTestDB(t)
	defer teardown()
	ctx := context.Background()

	ev1 := &domain.Event{Title: "Rock Fest", Category: "party", Price: 0, CreatedBy: "org_1"}
	ev2 := &domain.Event{Title: "Golang Meetup", Category: "education", Price: 100, CreatedBy: "org_2"}
	ev3 := &domain.Event{Title: "Python Basics", Category: "education", Price: 400, CreatedBy: "org_3"}

	_, err := repo.CreateEvent(ctx, ev1)
	assert.NoError(t, err)
	_, err = repo.CreateEvent(ctx, ev2)
	assert.NoError(t, err)
	_, err = repo.CreateEvent(ctx, ev3)
	assert.NoError(t, err)

	filters := map[string]string{
		"category": "education",
		"price_to": "200",
	}

	res, total, err := repo.QueryEvents(ctx, filters, 10, 0)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)

	if assert.Len(t, res, 1) {
		assert.Equal(t, "Golang Meetup", res[0].Title)
	}
}
