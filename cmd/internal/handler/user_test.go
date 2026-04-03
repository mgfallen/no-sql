package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"no-sql/cmd/internal/domain"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type MockUserProcessor struct {
	onRegister    func(fullName, username, password string) (*domain.User, error)
	onLogin       func(username, password string) (*domain.User, error)
	onCreateEvent func(event *domain.Event) (string, error)
	onListEvents  func(title string, limit, offset int64) ([]domain.Event, int64, error)
}

func (m *MockUserProcessor) Register(_ context.Context, f, u, p string) (*domain.User, error) {
	return m.onRegister(f, u, p)
}
func (m *MockUserProcessor) Login(_ context.Context, u, p string) (*domain.User, error) {
	return m.onLogin(u, p)
}
func (m *MockUserProcessor) CreateEvent(_ context.Context, e *domain.Event) (string, error) {
	return m.onCreateEvent(e)
}
func (m *MockUserProcessor) ListEvents(_ context.Context, t string, l, o int64) ([]domain.Event, int64, error) {
	return m.onListEvents(t, l, o)
}

type MockSessionManager struct {
	onGenerate    func() (string, error)
	onCheckExists func(sid string) (bool, error)
	onGetUserID   func(sid string) (string, error)
}

func (m *MockSessionManager) GenerateSID() (string, error) { return m.onGenerate() }
func (m *MockSessionManager) CheckExists(_ context.Context, s string) (bool, error) {
	return m.onCheckExists(s)
}
func (m *MockSessionManager) GetUserID(_ context.Context, s string) (string, error) {
	return m.onGetUserID(s)
}

type MockSessionStore struct {
	onCreate  func(sid string) (bool, error)
	onBind    func(sid, uid string) error
	onRefresh func(sid string) error
}

func (m *MockSessionStore) CreateSession(_ context.Context, s string) (bool, error) {
	return m.onCreate(s)
}
func (m *MockSessionStore) BindUser(_ context.Context, s, u string) error   { return m.onBind(s, u) }
func (m *MockSessionStore) RefreshTTL(_ context.Context, s string) error    { return m.onRefresh(s) }
func (m *MockSessionStore) DeleteSession(_ context.Context, s string) error { return nil }

func TestUserHandler_Register(t *testing.T) {
	t.Parallel()
	userID := bson.NewObjectID()

	tests := []struct {
		name           string
		body           string
		mockSetup      func(u *MockUserProcessor, m *MockSessionManager, s *MockSessionStore)
		expectedStatus int
		checkCookie    bool
	}{
		{
			name: "Success registration",
			body: `{"full_name": "Viktor", "username": "perov", "password": "password123"}`,
			mockSetup: func(u *MockUserProcessor, m *MockSessionManager, s *MockSessionStore) {
				u.onRegister = func(f, un, p string) (*domain.User, error) {
					return &domain.User{ID: userID, Username: un}, nil
				}
				m.onGenerate = func() (string, error) { return "new-session-id", nil }
				s.onCreate = func(sid string) (bool, error) { return true, nil }
				s.onBind = func(sid, uid string) error { return nil }
			},
			expectedStatus: http.StatusCreated,
			checkCookie:    true,
		},
		{
			name: "Conflict: user exists",
			body: `{"full_name": "Viktor", "username": "perov", "password": "password123"}`,
			mockSetup: func(u *MockUserProcessor, m *MockSessionManager, s *MockSessionStore) {
				u.onRegister = func(f, un, p string) (*domain.User, error) {
					return nil, mongo.WriteError{Code: 11000} // Исправлено на WriteError для дубликатов
				}
				m.onCheckExists = func(sid string) (bool, error) { return false, nil }
			},
			expectedStatus: http.StatusConflict,
			checkCookie:    false,
		},
		{
			name: "Bad Request: missing field",
			body: `{"username": "perov"}`,
			mockSetup: func(u *MockUserProcessor, m *MockSessionManager, s *MockSessionStore) {
				// Пустой мок, так как до вызова сервисов не дойдет
			},
			expectedStatus: http.StatusBadRequest,
			checkCookie:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uMock := &MockUserProcessor{}
			mMock := &MockSessionManager{}
			sMock := &MockSessionStore{}
			tt.mockSetup(uMock, mMock, sMock)

			h := NewUserHandler(uMock, mMock, sMock, 3600)

			req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			h.Register(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.checkCookie {
				cookies := rec.Result().Cookies()
				found := false
				for _, c := range cookies {
					if c.Name == "X-Session-Id" && c.Value == "new-session-id" {
						found = true
					}
				}
				assert.True(t, found)
			}
		})
	}
}

func TestUserHandler_Login(t *testing.T) {
	t.Parallel()
	userID := bson.NewObjectID()

	uMock := &MockUserProcessor{}
	mMock := &MockSessionManager{}
	sMock := &MockSessionStore{}

	h := NewUserHandler(uMock, mMock, sMock, 3600)

	t.Run("Success Login", func(t *testing.T) {
		body := `{"username": "perov", "password": "password123"}`

		uMock.onLogin = func(u, p string) (*domain.User, error) {
			return &domain.User{ID: userID, Username: u}, nil
		}
		mMock.onGenerate = func() (string, error) { return "login-session", nil }
		mMock.onCheckExists = func(sid string) (bool, error) { return false, nil }
		sMock.onCreate = func(sid string) (bool, error) { return true, nil }
		sMock.onBind = func(sid, uid string) error { return nil }
		sMock.onRefresh = func(sid string) error { return nil }

		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
		rec := httptest.NewRecorder()

		h.Login(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)
		cookies := rec.Result().Cookies()
		assert.NotEmpty(t, cookies)
		assert.Equal(t, "X-Session-Id", cookies[0].Name)
		assert.Equal(t, "login-session", cookies[0].Value)
	})
}
