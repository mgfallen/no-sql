package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"no-sql/cmd/internal/domain"
	"strconv"

	"go.mongodb.org/mongo-driver/mongo"
)

// UserProcessor описывает бизнес-логику пользователей и событий
type UserProcessor interface {
	Register(ctx context.Context, fullName, username, password string) (*domain.User, error)
	Login(ctx context.Context, username, password string) (*domain.User, error)
	CreateEvent(ctx context.Context, event *domain.Event) (string, error)
	ListEvents(ctx context.Context, title string, limit, offset int64) ([]domain.Event, int64, error)
}

// SessionManager описывает работу с сессиями (логика генерации и проверки)
type SessionManager interface {
	GenerateSID() (string, error)
	CheckExists(ctx context.Context, sid string) (bool, error)
	GetUserID(ctx context.Context, sid string) (string, error)
}

// SessionStore описывает прямое взаимодействие с Redis
type SessionStore interface {
	CreateSession(ctx context.Context, sid string) (bool, error)
	RefreshTTL(ctx context.Context, sid string) error
	BindUser(ctx context.Context, sid string, userID string) error
	DeleteSession(ctx context.Context, sid string) error
}

type UserHandler struct {
	userService    UserProcessor
	sessionService SessionManager
	sessionRepo    SessionStore
	sessionTTL     int
}

// NewUserHandler - конструктор со всеми зависимостями
func NewUserHandler(
	userService UserProcessor,
	sessionService SessionManager,
	sessionRepo SessionStore,
	ttl int,
) *UserHandler {
	return &UserHandler{
		userService:    userService,
		sessionService: sessionService,
		sessionRepo:    sessionRepo,
		sessionTTL:     ttl,
	}
}

// --- Методы Пользователей ---

// Register - POST /users
func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FullName string `json:"full_name"`
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleErrorWithSession(w, r, http.StatusBadRequest, "body")
		return
	}

	if req.FullName == "" || req.Username == "" || req.Password == "" {
		h.handleErrorWithSession(w, r, http.StatusBadRequest, "required fields missing")
		return
	}

	user, err := h.userService.Register(r.Context(), req.FullName, req.Username, req.Password)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			h.handleErrorWithSession(w, r, http.StatusConflict, "user already exists")
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	newSid, _ := h.sessionService.GenerateSID()
	_, _ = h.sessionRepo.CreateSession(r.Context(), newSid)
	_ = h.sessionRepo.BindUser(r.Context(), newSid, user.ID.Hex())

	h.setSessionCookie(w, newSid, h.sessionTTL)
	w.WriteHeader(http.StatusCreated)
}

// Login - POST /auth/login
func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	user, err := h.userService.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"message": "invalid credentials"})
		return
	}

	sid := ""
	if cookie, err := r.Cookie("X-Session-Id"); err == nil {
		sid = cookie.Value
	}

	exists, _ := h.sessionService.CheckExists(r.Context(), sid)
	if !exists {
		sid, _ = h.sessionService.GenerateSID()
		_, _ = h.sessionRepo.CreateSession(r.Context(), sid)
	}

	_ = h.sessionRepo.BindUser(r.Context(), sid, user.ID.Hex())
	_ = h.sessionRepo.RefreshTTL(r.Context(), sid)

	h.setSessionCookie(w, sid, h.sessionTTL)
	w.WriteHeader(http.StatusNoContent)
}

// Logout - POST /auth/logout
func (h *UserHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("X-Session-Id")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	sid := cookie.Value
	exists, _ := h.sessionService.CheckExists(r.Context(), sid)
	if !exists {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	uid, _ := h.sessionService.GetUserID(r.Context(), sid)
	if uid == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	_ = h.sessionRepo.DeleteSession(r.Context(), sid)

	h.setSessionCookie(w, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

// CreateEvent - POST /events
func (h *UserHandler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("X-Session-Id")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	uid, err := h.sessionService.GetUserID(r.Context(), cookie.Value)
	if err != nil || uid == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var req struct {
		Title       string `json:"title"`
		Address     string `json:"address"`
		StartedAt   string `json:"started_at"`
		FinishedAt  string `json:"finished_at"`
		Description string `json:"description"`
	}

	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleErrorWithSession(w, r, http.StatusBadRequest, "body")
		return
	}

	if req.Title == "" || req.Address == "" || req.StartedAt == "" || req.FinishedAt == "" {
		h.handleErrorWithSession(w, r, http.StatusBadRequest, "missing fields")
		return
	}

	event := &domain.Event{
		Title:       req.Title,
		Description: req.Description,
		Location:    domain.Location{Address: req.Address},
		CreatedBy:   uid,
		StartedAt:   req.StartedAt,
		FinishedAt:  req.FinishedAt,
	}

	id, err := h.userService.CreateEvent(r.Context(), event)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"message": "event already exists"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_ = h.sessionRepo.RefreshTTL(r.Context(), cookie.Value)
	h.setSessionCookie(w, cookie.Value, h.sessionTTL)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// ListEvents - GET /events
func (h *UserHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	title := q.Get("title")
	limit, _ := strconv.ParseInt(q.Get("limit"), 10, 64)
	offset, _ := strconv.ParseInt(q.Get("offset"), 10, 64)

	if limit == 0 {
		limit = 10
	}

	events, count, err := h.userService.ListEvents(r.Context(), title, limit, offset)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if cookie, err := r.Cookie("X-Session-Id"); err == nil {
		sid := cookie.Value
		if exists, _ := h.sessionService.CheckExists(r.Context(), sid); exists {
			_ = h.sessionRepo.RefreshTTL(r.Context(), sid)
			h.setSessionCookie(w, sid, h.sessionTTL)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"events": events,
		"count":  count,
	})
}

func (h *UserHandler) setSessionCookie(w http.ResponseWriter, sid string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     "X-Session-Id",
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   maxAge,
	})
}

func (h *UserHandler) handleErrorWithSession(w http.ResponseWriter, r *http.Request, status int, msg string) {
	if cookie, err := r.Cookie("X-Session-Id"); err == nil {
		exists, _ := h.sessionService.CheckExists(r.Context(), cookie.Value)
		if exists {
			_ = h.sessionRepo.RefreshTTL(r.Context(), cookie.Value)
			h.setSessionCookie(w, cookie.Value, h.sessionTTL)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"message": msg})
}
