package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockSessionRepo struct {
	mock.Mock
}

func (m *MockSessionRepo) CreateSession(ctx context.Context, sid string) (bool, error) {
	args := m.Called(ctx, sid)
	return args.Bool(0), args.Error(1)
}

func (m *MockSessionRepo) RefreshTTL(ctx context.Context, sid string) error {
	args := m.Called(ctx, sid)
	return args.Error(0)
}

func (m *MockSessionRepo) Exists(ctx context.Context, sid string) (bool, error) {
	args := m.Called(ctx, sid)
	return args.Bool(0), args.Error(1)
}

func (m *MockSessionRepo) GetUserIDBySession(ctx context.Context, sid string) (string, error) {
	args := m.Called(ctx, sid)
	return args.String(0), args.Error(1)
}

func (m *MockSessionRepo) BindUser(ctx context.Context, sid string, userID string) error {
	args := m.Called(ctx, sid, userID)
	return args.Error(0)
}

func (m *MockSessionRepo) DeleteSession(ctx context.Context, sid string) error {
	args := m.Called(ctx, sid)
	return args.Error(0)
}

func TestSessionService_GenerateSID(t *testing.T) {
	t.Parallel()
	repo := new(MockSessionRepo)
	svc := NewSessionService(repo)

	sid, err := svc.GenerateSID()
	assert.NoError(t, err)
	assert.Len(t, sid, 32) // 16 байт в hex-строке дают длину 32 символа
}

func TestSessionService_CheckExists(t *testing.T) {
	t.Parallel()

	t.Run("Empty SID", func(t *testing.T) {
		repo := new(MockSessionRepo)
		svc := NewSessionService(repo)

		exists, err := svc.CheckExists(context.Background(), "")
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("Valid SID - Exists", func(t *testing.T) {
		repo := new(MockSessionRepo)
		repo.On("Exists", mock.Anything, "session-123").Return(true, nil)
		svc := NewSessionService(repo)

		exists, err := svc.CheckExists(context.Background(), "session-123")
		assert.NoError(t, err)
		assert.True(t, exists)
		repo.AssertExpectations(t)
	})
}

func TestSessionService_GetUserID(t *testing.T) {
	t.Parallel()
	repo := new(MockSessionRepo)
	repo.On("GetUserIDBySession", mock.Anything, "session-123").Return("user-777", nil)
	svc := NewSessionService(repo)

	uid, err := svc.GetUserID(context.Background(), "session-123")
	assert.NoError(t, err)
	assert.Equal(t, "user-777", uid)

	// Проверка на пустую строку
	uidEmpty, errEmpty := svc.GetUserID(context.Background(), "")
	assert.NoError(t, errEmpty)
	assert.Empty(t, uidEmpty)
}

func TestSessionService_BindUser(t *testing.T) {
	t.Parallel()

	t.Run("Success Bind", func(t *testing.T) {
		repo := new(MockSessionRepo)
		repo.On("BindUser", mock.Anything, "session-123", "user-777").Return(nil)
		svc := NewSessionService(repo)

		err := svc.BindUser(context.Background(), "session-123", "user-777")
		assert.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("Empty Arguments Returns Error", func(t *testing.T) {
		repo := new(MockSessionRepo)
		svc := NewSessionService(repo)

		err := svc.BindUser(context.Background(), "", "user-777")
		assert.Error(t, err)

		err = svc.BindUser(context.Background(), "session-123", "")
		assert.Error(t, err)
	})
}

func TestSessionService_CreateSession(t *testing.T) {
	t.Parallel()

	t.Run("Success Create", func(t *testing.T) {
		repo := new(MockSessionRepo)
		repo.On("CreateSession", mock.Anything, "session-123").Return(true, nil)
		svc := NewSessionService(repo)

		ok, err := svc.CreateSession(context.Background(), "session-123")
		assert.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("Empty SID Returns Error", func(t *testing.T) {
		repo := new(MockSessionRepo)
		svc := NewSessionService(repo)

		ok, err := svc.CreateSession(context.Background(), "")
		assert.Error(t, err)
		assert.False(t, ok)
	})
}

func TestSessionService_RefreshTTL(t *testing.T) {
	t.Parallel()

	t.Run("Success Refresh", func(t *testing.T) {
		repo := new(MockSessionRepo)
		repo.On("RefreshTTL", mock.Anything, "session-123").Return(nil)
		svc := NewSessionService(repo)

		err := svc.RefreshTTL(context.Background(), "session-123")
		assert.NoError(t, err)
	})

	t.Run("Empty SID Handles Gracefully", func(t *testing.T) {
		repo := new(MockSessionRepo)
		svc := NewSessionService(repo)

		err := svc.RefreshTTL(context.Background(), "")
		assert.NoError(t, err)
	})
}

func TestSessionService_DeleteSession(t *testing.T) {
	t.Parallel()

	t.Run("Success Delete", func(t *testing.T) {
		repo := new(MockSessionRepo)
		repo.On("DeleteSession", mock.Anything, "session-123").Return(nil)
		svc := NewSessionService(repo)

		err := svc.DeleteSession(context.Background(), "session-123")
		assert.NoError(t, err)
	})

	t.Run("Empty SID Handles Gracefully", func(t *testing.T) {
		repo := new(MockSessionRepo)
		svc := NewSessionService(repo)

		err := svc.DeleteSession(context.Background(), "")
		assert.NoError(t, err)
	})
}

func TestSessionService_HandleSessionRequest(t *testing.T) {
	t.Parallel()

	t.Run("Existing Active Session", func(t *testing.T) {
		repo := new(MockSessionRepo)
		repo.On("Exists", mock.Anything, "active-sid").Return(true, nil)
		repo.On("RefreshTTL", mock.Anything, "active-sid").Return(nil)
		svc := NewSessionService(repo)

		sid, isNew, err := svc.HandleSessionRequest(context.Background(), "active-sid")
		assert.NoError(t, err)
		assert.Equal(t, "active-sid", sid)
		assert.False(t, isNew)
	})

	t.Run("Expired or New Session", func(t *testing.T) {
		repo := new(MockSessionRepo)
		repo.On("Exists", mock.Anything, "expired-sid").Return(false, nil)
		repo.On("CreateSession", mock.Anything, mock.MatchedBy(func(s string) bool {
			return len(s) == 32
		})).Return(true, nil)
		svc := NewSessionService(repo)

		sid, isNew, err := svc.HandleSessionRequest(context.Background(), "expired-sid")
		assert.NoError(t, err)
		assert.NotEmpty(t, sid)
		assert.True(t, isNew)
		assert.NotEqual(t, "expired-sid", sid)
	})

	t.Run("Redis Exists Error Proving Proliferation Prevention", func(t *testing.T) {
		repo := new(MockSessionRepo)
		// Имитируем критический сбой Redis
		repo.On("Exists", mock.Anything, "some-sid").Return(false, errors.New("redis connection timeout"))
		svc := NewSessionService(repo)

		// Теперь метод возвращает ошибку, а не плодит новую сессию
		sid, isNew, err := svc.HandleSessionRequest(context.Background(), "some-sid")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check existing session")
		assert.Empty(t, sid)
		assert.False(t, isNew)
	})

	t.Run("Redis Refresh Error Proving Failure Forwarding", func(t *testing.T) {
		repo := new(MockSessionRepo)
		repo.On("Exists", mock.Anything, "active-sid").Return(true, nil)
		repo.On("RefreshTTL", mock.Anything, "active-sid").Return(errors.New("redis write error"))
		svc := NewSessionService(repo)

		sid, isNew, err := svc.HandleSessionRequest(context.Background(), "active-sid")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to refresh session ttl")
		assert.Empty(t, sid)
		assert.False(t, isNew)
	})
}
