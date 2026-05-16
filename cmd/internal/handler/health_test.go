package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockSessionChecker struct {
	mock.Mock
}

func (m *MockSessionChecker) CheckExists(ctx context.Context, sid string) (bool, error) {
	args := m.Called(ctx, sid)
	return args.Bool(0), args.Error(1)
}

func TestHealthHandler(t *testing.T) {
	t.Parallel()

	const testTTL = 60
	const testSID = "3f8a2c1d9e4b7f0a5c6d2e8b1a3f9c7d"

	tests := []struct {
		name           string
		method         string
		cookie         *http.Cookie
		mockBehavior   func(m *MockSessionChecker)
		expectedStatus int
		expectedBody   map[string]string
		checkCookie    bool
	}{
		{
			name:           "1. GET /health - No Cookie - ok",
			method:         http.MethodGet,
			mockBehavior:   func(_ *MockSessionChecker) {},
			expectedStatus: http.StatusOK,
			expectedBody:   map[string]string{"status": "ok"},
			checkCookie:    false,
		},
		{
			name:   "2. GET /health - Valid Cookie - ok with Set-Cookie",
			method: http.MethodGet,
			cookie: &http.Cookie{Name: "X-Session-Id", Value: testSID},
			mockBehavior: func(m *MockSessionChecker) {
				m.On("CheckExists", mock.Anything, testSID).Return(true, nil)
			},
			expectedStatus: http.StatusOK,
			expectedBody:   map[string]string{"status": "ok"},
			checkCookie:    true,
		},
		{
			name:   "3. GET /health - Invalid/Expired Cookie - ok without Set-Cookie",
			method: http.MethodGet,
			cookie: &http.Cookie{Name: "X-Session-Id", Value: "wrong-sid"},
			mockBehavior: func(m *MockSessionChecker) {
				m.On("CheckExists", mock.Anything, "wrong-sid").Return(false, nil)
			},
			expectedStatus: http.StatusOK,
			expectedBody:   map[string]string{"status": "ok"},
			checkCookie:    false,
		},
		{
			name:           "4. POST /health - method not allowed",
			method:         http.MethodPost,
			mockBehavior:   func(_ *MockSessionChecker) {},
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockSvc := new(MockSessionChecker)
			tt.mockBehavior(mockSvc)
			h := NewHealthHandler(mockSvc, testTTL)

			req := httptest.NewRequest(tt.method, "/health", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			rec := httptest.NewRecorder()

			h.Health(rec, req)

			res := rec.Result()
			defer res.Body.Close()

			assert.Equal(t, tt.expectedStatus, res.StatusCode)

			if tt.expectedBody != nil {
				assert.Contains(t, res.Header.Get("Content-Type"), "application/json")
				var body map[string]string
				_ = json.NewDecoder(res.Body).Decode(&body)
				assert.Equal(t, tt.expectedBody, body)
			}

			if tt.checkCookie {
				cookies := res.Cookies()
				found := false
				for _, c := range cookies {
					if c.Name == "X-Session-Id" {
						assert.Equal(t, testSID, c.Value)
						assert.Equal(t, testTTL, c.MaxAge)
						found = true
					}
				}
				assert.True(t, found, "Expected X-Session-Id cookie not found")
			} else if tt.method == http.MethodGet {
				assert.Empty(t, res.Cookies())
			}

			mockSvc.AssertExpectations(t)
		})
	}
}
