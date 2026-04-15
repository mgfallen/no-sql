package service

import (
	"context"
	"errors"
	"no-sql/cmd/internal/domain"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/crypto/bcrypt"
)

// --- Mocks ---

type MockUserRepo struct{ mock.Mock }

func (m *MockUserRepo) CreateUser(ctx context.Context, u *domain.User) error {
	return m.Called(ctx, u).Error(0)
}
func (m *MockUserRepo) GetUserByUsername(ctx context.Context, un string) (*domain.User, error) {
	args := m.Called(ctx, un)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}
func (m *MockUserRepo) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.User), args.Error(1)
}
func (m *MockUserRepo) SearchUsers(ctx context.Context, n, id string, l, o int64) ([]domain.User, int64, error) {
	args := m.Called(ctx, n, id, l, o)
	return args.Get(0).([]domain.User), args.Get(1).(int64), args.Error(2)
}

type MockEventRepo struct{ mock.Mock }

func (m *MockEventRepo) CreateEvent(ctx context.Context, e *domain.Event) (string, error) {
	args := m.Called(ctx, e)
	return args.String(0), args.Error(1)
}
func (m *MockEventRepo) FindEvents(ctx context.Context, f map[string]interface{}, l, o int64) ([]domain.Event, int64, error) {
	args := m.Called(ctx, f, l, o)
	return args.Get(0).([]domain.Event), args.Get(1).(int64), args.Error(2)
}
func (m *MockEventRepo) GetEventByID(ctx context.Context, id string) (*domain.Event, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Event), args.Error(1)
}
func (m *MockEventRepo) UpdateEvent(ctx context.Context, eid, uid string, up bson.M) (bool, error) {
	args := m.Called(ctx, eid, uid, up)
	return args.Bool(0), args.Error(1)
}

// --- Helpers ---

func setup(t *testing.T) (*UserService, *MockUserRepo, *MockEventRepo) {
	u := new(MockUserRepo)
	e := new(MockEventRepo)
	return NewUserService(u, e), u, e
}

// --- Tests ---

func TestUserService_Register(t *testing.T) {
	t.Parallel()
	svc, uMock, _ := setup(t)

	tests := []struct {
		name     string
		username string
		mockErr  error
		wantErr  bool
	}{
		{"Success", "newuser", nil, false},
		{"Duplicate", "exists", errors.New("duplicate"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uMock.On("CreateUser", mock.Anything, mock.MatchedBy(func(u *domain.User) bool {
				return u.Username == tt.username
			})).Return(tt.mockErr).Once()

			user, err := svc.Register(context.Background(), "Name", tt.username, "pass")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.username, user.Username)
			}
		})
	}
}

func TestUserService_Login(t *testing.T) {
	t.Parallel()
	svc, uMock, _ := setup(t)
	pass := "secret"
	hash, _ := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	user := &domain.User{Username: "vic", PasswordHash: string(hash)}

	tests := []struct {
		name     string
		password string
		mockUser *domain.User
		mockErr  error
		wantErr  bool
	}{
		{"Valid", pass, user, nil, false},
		{"WrongPass", "wrong", user, nil, true},
		{"NotFound", pass, nil, errors.New("sql: no rows"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uMock.On("GetUserByUsername", mock.Anything, "vic").Return(tt.mockUser, tt.mockErr).Once()
			res, err := svc.Login(context.Background(), "vic", tt.password)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, res)
			}
		})
	}
}

func TestUserService_CreateEvent(t *testing.T) {
	t.Parallel()
	svc, _, eMock := setup(t)

	t.Run("Default Category", func(t *testing.T) {
		event := &domain.Event{Title: "Party"}
		eMock.On("CreateEvent", mock.Anything, mock.MatchedBy(func(e *domain.Event) bool {
			return e.Category == "other"
		})).Return("id123", nil).Once()

		id, err := svc.CreateEvent(context.Background(), event)
		assert.NoError(t, err)
		assert.Equal(t, "id123", id)
	})
}

func TestUserService_ListEvents(t *testing.T) {
	t.Parallel()
	svc, _, eMock := setup(t)
	filters := map[string]interface{}{"city": "Moscow"}

	tests := []struct {
		name   string
		events []domain.Event
		count  int64
		err    error
	}{
		{"Found", []domain.Event{{Title: "E1"}}, 1, nil},
		{"Empty", []domain.Event{}, 0, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eMock.On("FindEvents", mock.Anything, filters, int64(10), int64(0)).
				Return(tt.events, tt.count, tt.err).Once()

			res, count, err := svc.ListEvents(context.Background(), filters, 10, 0)
			assert.Equal(t, tt.count, count)
			assert.Len(t, res, len(tt.events))
			assert.NoError(t, err)
		})
	}
}

func TestUserService_PatchEvent(t *testing.T) {
	t.Parallel()
	svc, _, eMock := setup(t)
	updates := bson.M{"price": 100}

	tests := []struct {
		name  string
		found bool
		err   error
	}{
		{"Success", true, nil},
		{"ForbiddenOrNotFound", false, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eMock.On("UpdateEvent", mock.Anything, "eid", "uid", updates).Return(tt.found, tt.err).Once()
			ok, err := svc.PatchEvent(context.Background(), "eid", "uid", updates)
			assert.Equal(t, tt.found, ok)
			assert.NoError(t, err)
		})
	}
}

func TestUserService_FindUsers(t *testing.T) {
	t.Parallel()
	svc, uMock, _ := setup(t)

	t.Run("Search by name", func(t *testing.T) {
		t.Parallel()
		uMock.On("SearchUsers", mock.Anything, "Viktor", "", int64(10), int64(0)).
			Return([]domain.User{{FullName: "Viktor Perov"}}, int64(1), nil).Once()

		users, count, err := svc.FindUsers(context.Background(), "Viktor", "", 10, 0)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)
		assert.Equal(t, "Viktor Perov", users[0].FullName)
	})
}
