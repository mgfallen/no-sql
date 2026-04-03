package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockSessionProcessor struct {
	mock.Mock
}

func (m *MockSessionProcessor) HandleSessionRequest(ctx context.Context, sid string) (string, bool, error) {
	args := m.Called(ctx, sid)
	return args.String(0), args.Bool(1), args.Error(2)
}

func TestSessionHandler_ServeHTTP(t *testing.T) {
	t.Parallel()

	const testTTL = 60
	const oldSID = "old-session-id"
	const newSID = "new-crypto-secure-id"

	tests := []struct {
		name           string
		method         string
		cookie         *http.Cookie
		mockBehavior   func(m *MockSessionProcessor)
		expectedStatus int
		expectedSid    string
		expectedIsNew  bool
	}{
		{
			name:   "1. First visit - No cookie - 201 Created",
			method: http.MethodPost,
			cookie: nil,
			mockBehavior: func(m *MockSessionProcessor) {
				m.On("HandleSessionRequest", mock.Anything, "").Return(newSID, true, nil)
			},
			expectedStatus: http.StatusCreated,
			expectedSid:    newSID,
		},
		{
			name:   "2. Repeat visit - Valid cookie - 200 OK",
			method: http.MethodPost,
			cookie: &http.Cookie{Name: "X-Session-Id", Value: oldSID},
			mockBehavior: func(m *MockSessionProcessor) {
				m.On("HandleSessionRequest", mock.Anything, oldSID).Return(oldSID, false, nil)
			},
			expectedStatus: http.StatusOK,
			expectedSid:    oldSID,
		},
		{
			name:   "3. Invalid cookie - Expired in DB - 201 Created with new ID",
			method: http.MethodPost,
			cookie: &http.Cookie{Name: "X-Session-Id", Value: "expired-id"},
			mockBehavior: func(m *MockSessionProcessor) {
				m.On("HandleSessionRequest", mock.Anything, "expired-id").Return(newSID, true, nil)
			},
			expectedStatus: http.StatusCreated,
			expectedSid:    newSID,
		},
		{
			name:           "4. Wrong Method - GET - 405 Method Not Allowed",
			method:         http.MethodGet,
			mockBehavior:   func(m *MockSessionProcessor) {},
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockSvc := new(MockSessionProcessor)
			tt.mockBehavior(mockSvc)
			h := NewSessionHandler(mockSvc, testTTL)

			req := httptest.NewRequest(tt.method, "/session", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			res := rec.Result()
			defer func(Body io.ReadCloser) {
				err := Body.Close()
				if err != nil {
					t.Fatal(err)
				}
			}(res.Body)

			assert.Equal(t, tt.expectedStatus, res.StatusCode)

			if tt.expectedStatus == http.StatusOK || tt.expectedStatus == http.StatusCreated {
				assert.Equal(t, "0", res.Header.Get("Content-Length"))

				cookies := res.Cookies()
				requireCookie(t, cookies, tt.expectedSid, testTTL)
			}

			mockSvc.AssertExpectations(t)
		})
	}
}

// Helper для проверки куки
func requireCookie(t *testing.T, cookies []*http.Cookie, expectedValue string, expectedTTL int) {
	t.Helper()
	var found bool
	for _, c := range cookies {
		if c.Name == "X-Session-Id" {
			assert.Equal(t, expectedValue, c.Value)
			assert.Equal(t, expectedTTL, c.MaxAge)
			assert.True(t, c.HttpOnly)
			assert.Equal(t, "/", c.Path)
			found = true
		}
	}
	assert.True(t, found, "X-Session-Id cookie not found in response")
}
