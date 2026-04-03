package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"no-sql/cmd/internal/domain"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/mongo"
)

// MockUserService имитирует бизнес-логику
type MockUserService struct {
	onCreate func(user *domain.User) error
	onGet    func(username string) (*domain.User, error)
}

func (m *MockUserService) CreateUser(_ context.Context, user *domain.User) error {
	return m.onCreate(user)
}

func (m *MockUserService) GetUserByUsername(_ context.Context, username string) (*domain.User, error) {
	return m.onGet(username)
}

func TestUserHandler_Register(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		body           string
		mockBehavior   func(m *MockUserService)
		expectedStatus int
	}{
		{
			name: "Success registration",
			body: `{"username": "new_user", "full_name": "Test"}`,
			mockBehavior: func(m *MockUserService) {
				m.onCreate = func(u *domain.User) error { return nil }
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "Conflict: user exists",
			body: `{"username": "existing"}`,
			mockBehavior: func(m *MockUserService) {
				m.onCreate = func(u *domain.User) error {
					return mongo.WriteError{Code: 11000}
				}
			},
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "Bad Request: invalid JSON",
			body:           `{invalid}`,
			mockBehavior:   func(m *MockUserService) {},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockSvc := &MockUserService{}
			tt.mockBehavior(mockSvc)
			h := NewUserHandler(mockSvc)

			req := httptest.NewRequest(http.MethodPost, "/user/register", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			h.Register(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
		})
	}
}

func TestUserHandler_GetProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		cookie         *http.Cookie
		mockBehavior   func(m *MockUserService)
		expectedStatus int
		checkBody      bool
	}{
		{
			name:   "Success: get profile",
			cookie: &http.Cookie{Name: "X-Session-Id", Value: "active_user"},
			mockBehavior: func(m *MockUserService) {
				m.onGet = func(username string) (*domain.User, error) {
					return &domain.User{Username: username, FullName: "Active"}, nil
				}
			},
			expectedStatus: http.StatusOK,
			checkBody:      true,
		},
		{
			name:           "Unauthorized: no cookie",
			cookie:         nil,
			mockBehavior:   func(m *MockUserService) {},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:   "Not Found: user vanished",
			cookie: &http.Cookie{Name: "X-Session-Id", Value: "ghost"},
			mockBehavior: func(m *MockUserService) {
				m.onGet = func(username string) (*domain.User, error) {
					return nil, mongo.ErrNoDocuments
				}
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockSvc := &MockUserService{}
			tt.mockBehavior(mockSvc)
			h := NewUserHandler(mockSvc)

			req := httptest.NewRequest(http.MethodGet, "/user/profile", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			rec := httptest.NewRecorder()

			h.GetProfile(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)

			if tt.checkBody {
				var u domain.User
				err := json.NewDecoder(rec.Body).Decode(&u)
				assert.NoError(t, err)
				assert.NotEmpty(t, u.Username)
			}
		})
	}
}
