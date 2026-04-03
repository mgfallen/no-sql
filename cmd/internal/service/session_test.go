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
					return len(s) == 32
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
		{
			name:        "New session: provided SID expired in DB",
			existingSid: "expired-sid",
			mockSetup: func(m *MockSessionRepo) {
				m.On("Exists", mock.Anything, "expired-sid").Return(false, nil)
				m.On("CreateSession", mock.Anything, mock.Anything).Return(true, nil)
			},
			expectedIsNew: true,
			expectedError: false,
		},
		{
			name:        "Error: DB failure on Exists",
			existingSid: "some-sid",
			mockSetup: func(m *MockSessionRepo) {
				m.On("Exists", mock.Anything, "some-sid").Return(false, errors.New("db down"))
				m.On("CreateSession", mock.Anything, mock.Anything).Return(true, nil)
			},
			expectedIsNew: true,
			expectedError: false,
		},
		{
			name:        "Error: DB failure on RefreshTTL",
			existingSid: "valid-sid",
			mockSetup: func(m *MockSessionRepo) {
				m.On("Exists", mock.Anything, "valid-sid").Return(true, nil)
				m.On("RefreshTTL", mock.Anything, "valid-sid").Return(errors.New("refresh failed"))
			},
			expectedIsNew: false,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

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

func TestSessionService_CheckExists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		sid            string
		mockSetup      func(m *MockSessionRepo)
		expectedResult bool
	}{
		{
			name:           "Empty SID",
			sid:            "",
			mockSetup:      func(_ *MockSessionRepo) {},
			expectedResult: false,
		},
		{
			name: "SID exists in DB",
			sid:  "exists",
			mockSetup: func(m *MockSessionRepo) {
				m.On("Exists", mock.Anything, "exists").Return(true, nil)
			},
			expectedResult: true,
		},
		{
			name: "SID missing in DB",
			sid:  "missing",
			mockSetup: func(m *MockSessionRepo) {
				m.On("Exists", mock.Anything, "missing").Return(false, nil)
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := new(MockSessionRepo)
			tt.mockSetup(repo)
			svc := NewSessionService(repo)

			result, err := svc.CheckExists(context.Background(), tt.sid)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedResult, result)
			repo.AssertExpectations(t)
		})
	}
}
