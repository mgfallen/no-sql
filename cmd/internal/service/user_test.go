package service

import (
	"context"
	"errors"
	"testing"

	"no-sql/cmd/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/crypto/bcrypt"
)

type MockUserRepo struct {
	mock.Mock
}

func (m *MockUserRepo) CreateUser(ctx context.Context, user *domain.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockUserRepo) GetUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	args := m.Called(ctx, username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}

func (m *MockUserRepo) CreateEvent(ctx context.Context, event *domain.Event) (string, error) {
	args := m.Called(ctx, event)
	return args.String(0), args.Error(1)
}

// Обновленный метод мока: теперь принимает map[string]string
func (m *MockUserRepo) QueryEvents(ctx context.Context, filters map[string]string, limit, offset int64) ([]domain.Event, int64, error) {
	args := m.Called(ctx, filters, limit, offset)

	var events []domain.Event
	if args.Get(0) != nil {
		events = args.Get(0).([]domain.Event)
	}
	return events, args.Get(1).(int64), args.Error(2)
}

func (m *MockUserRepo) UpdateEvent(ctx context.Context, id string, createdBy string, category string, price *uint64, city *string) error {
	args := m.Called(ctx, id, createdBy, category, price, city)
	return args.Error(0)
}

func setupUserService() (*UserService, *MockUserRepo) {
	mockRepo := new(MockUserRepo)
	svc := NewUserService(mockRepo)
	return svc, mockRepo
}

func TestUserService_CreateEvent(t *testing.T) {
	t.Parallel()
	svc, mockRepo := setupUserService()

	event := &domain.Event{Title: "Party"}
	mockRepo.On("CreateEvent", mock.Anything, mock.Anything).Return("event-id-123", nil)

	id, err := svc.CreateEvent(context.Background(), event)

	assert.NoError(t, err)
	assert.Equal(t, "event-id-123", id)
	assert.NotEmpty(t, event.CreatedAt)
	mockRepo.AssertExpectations(t)
}

func TestUserService_ListEvents(t *testing.T) {
	t.Parallel()
	svc, mockRepo := setupUserService()

	expectedEvents := []domain.Event{{Title: "E1"}}

	// Подготавливаем мапу фильтров, которую хендлер передает в сервис
	testFilters := map[string]string{
		"title":    "test",
		"category": "party",
		"city":     "Frankfurt",
	}

	// Мок ожидает чистую мапу map[string]string, построение bson.M переехало в репозиторий
	mockRepo.On("QueryEvents", mock.Anything, testFilters, int64(10), int64(0)).
		Return(expectedEvents, int64(1), nil)

	events, count, err := svc.ListEvents(context.Background(), testFilters, 10, 0)

	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)
	assert.Len(t, events, 1)
	mockRepo.AssertExpectations(t)
}

func TestUserService_UpdateEvent(t *testing.T) {
	t.Parallel()
	svc, mockRepo := setupUserService()

	price := uint64(500)
	city := "Frankfurt"

	mockRepo.On("UpdateEvent", mock.Anything, "ev-1", "usr-1", "concert",
		mock.MatchedBy(func(p *uint64) bool { return p != nil && *p == price }),
		mock.MatchedBy(func(c *string) bool { return c != nil && *c == city }),
	).Return(nil)

	err := svc.UpdateEvent(context.Background(), "ev-1", "usr-1", "concert", &price, &city)
	assert.NoError(t, err)

	// Тест на валидацию пустых полей в сервисе
	err = svc.UpdateEvent(context.Background(), "", "usr-1", "concert", nil, nil)
	assert.Error(t, err)

	mockRepo.AssertExpectations(t)
}

func TestUserService_Register(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fullName string
		username string
		password string
		mockErr  error
		wantErr  bool
	}{
		{
			name:     "Success registration",
			fullName: "John Doe",
			username: "johndoe",
			password: "password123",
			mockErr:  nil,
			wantErr:  false,
		},
		{
			name:     "User already exists",
			fullName: "John Doe",
			username: "exists",
			password: "password123",
			mockErr:  errors.New("mongo: duplicate key"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, mockRepo := setupUserService()

			mockRepo.On("CreateUser", mock.Anything, mock.MatchedBy(func(u *domain.User) bool {
				return u.Username == tt.username && u.FullName == tt.fullName
			})).Return(tt.mockErr)

			user, err := svc.Register(context.Background(), tt.fullName, tt.username, tt.password)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, tt.username, user.Username)
				err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(tt.password))
				assert.NoError(t, err, "Password should be correctly hashed")
			}
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestUserService_Login(t *testing.T) {
	t.Parallel()

	correctPassword := "svp4_pass"
	hash, _ := bcrypt.GenerateFromPassword([]byte(correctPassword), bcrypt.DefaultCost)

	objID, err := bson.ObjectIDFromHex("507f1f77bcf86cd799439011")
	if err != nil {
		t.Fatalf("failed to create objectID: %v", err)
	}

	existingUser := &domain.User{
		ID:           objID,
		Username:     "j0hnd0e42",
		PasswordHash: string(hash),
	}

	tests := []struct {
		name     string
		username string
		password string
		mockUser *domain.User
		mockErr  error
		wantErr  bool
	}{
		{
			name:     "Successful login",
			username: "j0hnd0e42",
			password: correctPassword,
			mockUser: existingUser,
			mockErr:  nil,
			wantErr:  false,
		},
		{
			name:     "User not found",
			username: "stranger",
			password: "any",
			mockUser: nil,
			mockErr:  errors.New("not found"),
			wantErr:  true,
		},
		{
			name:     "Wrong password",
			username: "j0hnd0e42",
			password: "incorrect_password",
			mockUser: existingUser,
			mockErr:  nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, mockRepo := setupUserService()

			mockRepo.On("GetUserByUsername", mock.Anything, tt.username).Return(tt.mockUser, tt.mockErr)
			user, err := svc.Login(context.Background(), tt.username, tt.password)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, tt.username, user.Username)
				assert.Equal(t, tt.mockUser.ID, user.ID)
			}
			mockRepo.AssertExpectations(t)
		})
	}
}
