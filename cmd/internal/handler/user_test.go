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
	"go.mongodb.org/mongo-driver/v2/bson"
)

type MockUserProcessor struct {
	onRegister     func(fullName, username, password string) (*domain.User, error)
	onLogin        func(username, password string) (*domain.User, error)
	onCreateEvent  func(event *domain.Event) (string, error)
	onListEvents   func(filters map[string]interface{}, limit, offset int64) ([]domain.Event, int64, error)
	onGetEventByID func(id string) (*domain.Event, error)
	onPatchEvent   func(eventID string, userID string, updates bson.M) (bool, error)
	onFindUsers    func(name, id string, limit, offset int64) ([]domain.User, int64, error)
	onGetUserByID  func(id string) (*domain.User, error)
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
func (m *MockUserProcessor) ListEvents(_ context.Context, f map[string]interface{}, l, o int64) ([]domain.Event, int64, error) {
	return m.onListEvents(f, l, o)
}
func (m *MockUserProcessor) GetEventByID(_ context.Context, id string) (*domain.Event, error) {
	return m.onGetEventByID(id)
}
func (m *MockUserProcessor) PatchEvent(_ context.Context, eid, uid string, up bson.M) (bool, error) {
	return m.onPatchEvent(eid, uid, up)
}
func (m *MockUserProcessor) FindUsers(_ context.Context, n, id string, l, o int64) ([]domain.User, int64, error) {
	return m.onFindUsers(n, id, l, o)
}
func (m *MockUserProcessor) GetUserByID(_ context.Context, id string) (*domain.User, error) {
	return m.onGetUserByID(id)
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
	onDelete  func(sid string) error
}

func (m *MockSessionStore) CreateSession(_ context.Context, s string) (bool, error) {
	return m.onCreate(s)
}
func (m *MockSessionStore) BindUser(_ context.Context, s, u string) error   { return m.onBind(s, u) }
func (m *MockSessionStore) RefreshTTL(_ context.Context, s string) error    { return m.onRefresh(s) }
func (m *MockSessionStore) DeleteSession(_ context.Context, s string) error { return m.onDelete(s) }

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
					return nil, mongo.WriteError{Code: 11000}
				}
				m.onCheckExists = func(sid string) (bool, error) { return false, nil }
			},
			expectedStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			uMock, mMock, sMock := &MockUserProcessor{}, &MockSessionManager{}, &MockSessionStore{}
			tt.mockSetup(uMock, mMock, sMock)
			h := NewUserHandler(uMock, mMock, sMock, 3600)

			req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			h.Register(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
		})
	}
}

func TestUserHandler_UpdateEvent(t *testing.T) {
	t.Parallel()
	uMock, mMock, sMock := &MockUserProcessor{}, &MockSessionManager{}, &MockSessionStore{}
	h := NewUserHandler(uMock, mMock, sMock, 3600)

	t.Run("Success Patch", func(t *testing.T) {
		t.Parallel()
		eventID := "65e9c0b1a2b3c4d5e6f7a8b7"
		body := `{"category": "party", "price": 1000, "city": "Moscow"}`

		mMock.onGetUserID = func(sid string) (string, error) { return "user123", nil }
		uMock.onPatchEvent = func(eid, uid string, up bson.M) (bool, error) {
			assert.Equal(t, "user123", uid)
			assert.Equal(t, uint(1000), up["price"])
			assert.Equal(t, "party", up["category"])
			return true, nil
		}

		req := httptest.NewRequest(http.MethodPatch, "/events/"+eventID, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: "X-Session-Id", Value: "valid-sid"})
		rec := httptest.NewRecorder()

		h.UpdateEvent(rec, req)
		assert.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("Invalid Category 400", func(t *testing.T) {
		t.Parallel()
		body := `{"category": "work"}`
		mMock.onGetUserID = func(sid string) (string, error) { return "user123", nil }

		req := httptest.NewRequest(http.MethodPatch, "/events/123", strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: "X-Session-Id", Value: "sid"})
		rec := httptest.NewRecorder()

		h.UpdateEvent(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestUserHandler_ListEvents(t *testing.T) {
	t.Parallel()
	uMock, mMock, sMock := &MockUserProcessor{}, &MockSessionManager{}, &MockSessionStore{}
	h := NewUserHandler(uMock, mMock, sMock, 3600)

	t.Run("Filter by category and price", func(t *testing.T) {
		t.Parallel()
		uMock.onListEvents = func(filters map[string]interface{}, l, o int64) ([]domain.Event, int64, error) {
			assert.Equal(t, "concert", filters["category"])
			assert.Equal(t, uint(500), filters["price_from"])
			return []domain.Event{{Title: "Rock Fest"}}, 1, nil
		}
		mMock.onCheckExists = func(sid string) (bool, error) { return false, nil }

		req := httptest.NewRequest(http.MethodGet, "/events?category=concert&price_from=500", nil)
		rec := httptest.NewRecorder()

		h.ListEvents(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var resp map[string]interface{}
		json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.Equal(t, float64(1), resp["count"])
	})
}

func TestUserHandler_ListUsers(t *testing.T) {
	t.Parallel()
	uMock, mMock, sMock := &MockUserProcessor{}, &MockSessionManager{}, &MockSessionStore{}
	h := NewUserHandler(uMock, mMock, sMock, 3600)

	t.Run("Search by name", func(t *testing.T) {
		t.Parallel()
		uMock.onFindUsers = func(name, id string, l, o int64) ([]domain.User, int64, error) {
			assert.Equal(t, "Ivan", name)
			return []domain.User{{FullName: "Ivan Ivanov"}}, 1, nil
		}
		mMock.onCheckExists = func(sid string) (bool, error) { return false, nil }

		req := httptest.NewRequest(http.MethodGet, "/users?name=Ivan", nil)
		rec := httptest.NewRecorder()

		h.ListUsers(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "Ivan Ivanov")
	})
}

func TestUserHandler_GetEvent(t *testing.T) {
	t.Parallel()
	uMock := &MockUserProcessor{}
	h := NewUserHandler(uMock, &MockSessionManager{}, &MockSessionStore{}, 3600)

	t.Run("Event Not Found 404", func(t *testing.T) {
		t.Parallel()
		uMock.onGetEventByID = func(id string) (*domain.Event, error) {
			return nil, mongo.ErrNoDocuments
		}
		req := httptest.NewRequest(http.MethodGet, "/events/missing", nil)
		rec := httptest.NewRecorder()

		h.GetEvent(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}
