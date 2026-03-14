package handler

import (
	"context"
	"encoding/json"
	"net/http"
)

type SessionChecker interface {
	CheckExists(ctx context.Context, sid string) (bool, error)
}

type HealthHandler struct {
	service    SessionChecker
	sessionTTL int
}

func NewHealthHandler(svc SessionChecker, ttl int) *HealthHandler {
	return &HealthHandler{
		service:    svc,
		sessionTTL: ttl,
	}
}

func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if cookie, err := r.Cookie("X-Session-Id"); err == nil {
		exists, _ := h.service.CheckExists(r.Context(), cookie.Value)

		if exists {
			http.SetCookie(w, &http.Cookie{
				Name:     "X-Session-Id",
				Value:    cookie.Value,
				Path:     "/",
				HttpOnly: true,
				MaxAge:   h.sessionTTL,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
