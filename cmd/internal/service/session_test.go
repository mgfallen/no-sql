package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockSessionRepo struct {
	mock.Mock
}

// Реализуем новый метод интерфейса
func (m *MockSessionRepo) GetUserIDBySession(ctx context.Context, sid string) (string, error) {
	args := m.Called(ctx, sid)
	return args.String(0), args.Error(1)
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

func TestSessionService_GetUserID(t *testing.T) {
	t.Parallel()
	repo := new(MockSessionRepo)
	svc := NewSessionService(repo)

	repo.On("GetUserIDBySession", mock.Anything, "active-sid").Return("user-123", nil)

	uid, err := svc.GetUserID(context.Background(), "active-sid")

	assert.NoError(t, err)
	assert.Equal(t, "user-123", uid)
	repo.AssertExpectations(t)
}

func TestSessionService_HandleSessionRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		existingSid   string
		mockSetup     func(m *MockSessionRepo)
		expectedIsNew bool
		expectedError bool
	}{
		{
			name:        "New session: empty SID provided",
			existingSid: "",
			mockSetup: func(m *MockSessionRepo) {
				m.On("CreateSession", mock.Anything, mock.MatchedBy(func(s string) bool {
					return len(s) == 32 // hex от 16 байт
				})).Return(true, nil)
			},
			expectedIsNew: true,
			expectedError: false,
		},
		{
			name:        "Existing session: valid SID provided",
			existingSid: "valid-sid",
			mockSetup: func(m *MockSessionRepo) {
				m.On("Exists", mock.Anything, "valid-sid").Return(true, nil)
				m.On("RefreshTTL", mock.Anything, "valid-sid").Return(nil)
			},
			expectedIsNew: false,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockSessionRepo)
			tt.mockSetup(repo)
			svc := NewSessionService(repo)

			sid, isNew, err := svc.HandleSessionRequest(context.Background(), tt.existingSid)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, sid)
				assert.Equal(t, tt.expectedIsNew, isNew)
			}
			repo.AssertExpectations(t)
		})
	}
}
