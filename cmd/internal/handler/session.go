package handler

import (
	"context"
	"net/http"
)

// SessionProcessor - интерфейс сервиса
type SessionProcessor interface {
	HandleSessionRequest(ctx context.Context, sid string) (finalSid string, isNew bool, err error)
}

// SessionHandler - реализация хендлера
type SessionHandler struct {
	service    SessionProcessor
	sessionTTL int
}

// NewSessionHandler - конструктор
func NewSessionHandler(svc SessionProcessor, ttl int) *SessionHandler {
	return &SessionHandler{
		service:    svc,
		sessionTTL: ttl,
	}
}

// ServeHTTP - обработать HTTP запрос
func (h *SessionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var sid string
	if cookie, err := r.Cookie("X-Session-Id"); err == nil {
		sid = cookie.Value
	}

	finalSid, isNew, err := h.service.HandleSessionRequest(r.Context(), sid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "X-Session-Id",
		Value:    finalSid,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   h.sessionTTL,
	})

	w.Header().Set("Content-Length", "0")
	if isNew {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}
