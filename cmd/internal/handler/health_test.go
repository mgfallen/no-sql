package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHealthHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		method         string
		expectedStatus int
		expectedBody   map[string]string
		checkJSON      bool
	}{
		{
			name:           "1. GET /health - ok",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedBody:   map[string]string{"status": "ok"},
			checkJSON:      true,
		},
		{
			name:           "2. POST /health - method not allowed",
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "3. PUT /health - method not allowed",
			method:         http.MethodPut,
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tt.method, "/health", nil)
			rec := httptest.NewRecorder()

			h := NewHealthHandler()
			h.Health(rec, req)

			res := rec.Result()
			defer func(body io.ReadCloser) {
				_ = body.Close()
			}(res.Body)

			assert.Equal(t, tt.expectedStatus, res.StatusCode)
			if tt.checkJSON {
				assert.Contains(t, res.Header.Get("Content-Type"), "application/json")

				var body map[string]string
				err := json.NewDecoder(res.Body).Decode(&body)
				assert.NoError(t, err)

				assert.Equal(t, tt.expectedBody, body)
			}
		})
	}
}
