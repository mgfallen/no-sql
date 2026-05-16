package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"no-sql/cmd/internal/domain"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type MockUserProcessor struct {
	onRegister    func(fullName, username, password string) (*domain.User, error)
	onLogin       func(username, password string) (*domain.User, error)
	onCreateEvent func(event *domain.Event) (string, error)
	onListEvents  func(filters map[string]string, limit, offset int64) ([]domain.Event, int64, error)
	onUpdateEvent func(id string, createdBy string, category string, price *uint64, city *string) error
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
func (m *MockUserProcessor) ListEvents(_ context.Context, filters map[string]string, l, o int64) ([]domain.Event, int64, error) {
	return m.onListEvents(filters, l, o)
}
func (m *MockUserProcessor) UpdateEvent(_ context.Context, id, cb, cat string, p *uint64, c *string) error {
	return m.onUpdateEvent(id, cb, cat, p, c)
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
func (m *MockSessionStore) DeleteSession(_ context.Context, _ string) error { return nil }

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
				u.onRegister = func(_, un, _ string) (*domain.User, error) {
					return &domain.User{ID: userID, Username: un}, nil
				}
				m.onGenerate = func() (string, error) { return "new-session-id", nil }
				s.onCreate = func(_ string) (bool, error) { return true, nil }
				s.onBind = func(_, _ string) error { return nil }
			},
			expectedStatus: http.StatusCreated,
			checkCookie:    true,
		},
		{
			name: "Conflict: user exists",
			body: `{"full_name": "Viktor", "username": "perov", "password": "password123"}`,
			mockSetup: func(u *MockUserProcessor, m *MockSessionManager, s *MockSessionStore) {
				u.onRegister = func(_, _, _ string) (*domain.User, error) {
					return nil, mongo.WriteException{
						WriteErrors: []mongo.WriteError{{Code: 11000, Message: "duplicate key error"}},
					}
				}
				m.onCheckExists = func(_ string) (bool, error) { return false, nil }
			},
			expectedStatus: http.StatusConflict,
			checkCookie:    false,
		},
		{
			name: "Bad Request: missing field",
			body: `{"username": "perov"}`,
			mockSetup: func(_ *MockUserProcessor, _ *MockSessionManager, _ *MockSessionStore) {
				// Пусто
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

		uMock.onLogin = func(u, _ string) (*domain.User, error) {
			return &domain.User{ID: userID, Username: u}, nil
		}
		mMock.onGenerate = func() (string, error) { return "login-session", nil }
		mMock.onCheckExists = func(_ string) (bool, error) { return false, nil }
		sMock.onCreate = func(_ string) (bool, error) { return true, nil }
		sMock.onBind = func(_, _ string) error { return nil }
		sMock.onRefresh = func(_ string) error { return nil }

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

func TestUserHandler_UpdateEvent(t *testing.T) {
	t.Parallel()

	uMock := &MockUserProcessor{}
	mMock := &MockSessionManager{}
	sMock := &MockSessionStore{}

	h := NewUserHandler(uMock, mMock, sMock, 3600)

	t.Run("Success Patch Event", func(t *testing.T) {
		body := `{"category": "party", "price": 1500}`

		mMock.onGetUserID = func(_ string) (string, error) { return "user-111", nil }

		// На этапе предпроверки возвращаем событие, где CreatedBy равен текущему юзеру "user-111"
		uMock.onListEvents = func(filters map[string]string, _, _ int64) ([]domain.Event, int64, error) {
			assert.Equal(t, "event-999", filters["id"])
			return []domain.Event{
				{CreatedBy: "user-111"},
			}, 1, nil
		}

		uMock.onUpdateEvent = func(id, cb, cat string, p *uint64, _ *string) error {
			assert.Equal(t, "event-999", id)
			assert.Equal(t, "user-111", cb)
			assert.Equal(t, "party", cat)
			assert.Equal(t, uint64(1500), *p)
			return nil
		}
		sMock.onRefresh = func(_ string) error { return nil }

		req := httptest.NewRequest(http.MethodPatch, "/events/event-999", strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: "X-Session-Id", Value: "valid-session"})
		rec := httptest.NewRecorder()

		h.UpdateEvent(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("Invalid Category Returns 400", func(t *testing.T) {
		body := `{"category": "invalid-category-name"}`

		mMock.onGetUserID = func(_ string) (string, error) { return "user-111", nil }

		req := httptest.NewRequest(http.MethodPatch, "/events/event-999", strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: "X-Session-Id", Value: "valid-session"})
		rec := httptest.NewRecorder()

		h.UpdateEvent(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp map[string]string
		err := json.NewDecoder(rec.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Equal(t, "invalid \"category\" field", resp["message"])
	})

	t.Run("Event Not Found Returns 404", func(t *testing.T) {
		body := `{"category": "meetup"}`

		mMock.onGetUserID = func(_ string) (string, error) { return "user-111", nil }

		// Эмулируем отсутствие события в БД
		uMock.onListEvents = func(_ map[string]string, _, _ int64) ([]domain.Event, int64, error) {
			return nil, 0, nil
		}

		req := httptest.NewRequest(http.MethodPatch, "/events/event-absent", strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: "X-Session-Id", Value: "valid-session"})
		rec := httptest.NewRecorder()

		h.UpdateEvent(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp map[string]string
		err := json.NewDecoder(rec.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Equal(t, "event not found", resp["message"])
	})

	t.Run("Forbidden: User Is Not The Organizer Returns 403", func(t *testing.T) {
		body := `{"category": "meetup"}`

		mMock.onGetUserID = func(_ string) (string, error) { return "user-111", nil }

		// Возвращаем чужое событие (CreatedBy = "user-another")
		uMock.onListEvents = func(_ map[string]string, _, _ int64) ([]domain.Event, int64, error) {
			return []domain.Event{
				{CreatedBy: "user-another"},
			}, 1, nil
		}

		req := httptest.NewRequest(http.MethodPatch, "/events/event-foreign", strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: "X-Session-Id", Value: "valid-session"})
		rec := httptest.NewRecorder()

		h.UpdateEvent(rec, req)

		assert.Equal(t, http.StatusForbidden, rec.Code)

		var resp map[string]string
		err := json.NewDecoder(rec.Body).Decode(&resp)
		assert.NoError(t, err)
		assert.Equal(t, "you are not the organizer of this event", resp["message"])
	})
}
