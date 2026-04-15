package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"no-sql/cmd/internal/domain"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// UserProcessor описывает бизнес-логику пользователей и событий
type UserProcessor interface {
	Register(ctx context.Context, fullName, username, password string) (*domain.User, error)
	Login(ctx context.Context, username, password string) (*domain.User, error)

	// События
	CreateEvent(ctx context.Context, event *domain.Event) (string, error)
	ListEvents(ctx context.Context, filters map[string]interface{}, limit, offset int64) ([]domain.Event, int64, error)
	GetEventByID(ctx context.Context, id string) (*domain.Event, error)
	PatchEvent(ctx context.Context, eventID string, userID string, updates bson.M) (bool, error)

	// Пользователи (Организаторы)
	FindUsers(ctx context.Context, name, id string, limit, offset int64) ([]domain.User, int64, error)
	GetUserByID(ctx context.Context, id string) (*domain.User, error)
}

// SessionManager описывает работу с сессиями
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

func NewUserHandler(usp UserProcessor, sm SessionManager, ss SessionStore, ttl int) *UserHandler {
	return &UserHandler{
		userService:    usp,
		sessionService: sm,
		sessionRepo:    ss,
		sessionTTL:     ttl,
	}
}

// --- Управление сессиями и ошибками ---

func (h *UserHandler) setSessionCookie(w http.ResponseWriter, sid string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     "X-Session-Id",
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   maxAge,
	})
}

func (h *UserHandler) refreshSessionIfExists(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("X-Session-Id"); err == nil {
		sid := cookie.Value
		if exists, _ := h.sessionService.CheckExists(r.Context(), sid); exists {
			_ = h.sessionRepo.RefreshTTL(r.Context(), sid)
			h.setSessionCookie(w, sid, h.sessionTTL)
		}
	}
}

func (h *UserHandler) handleError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"message": msg})
}

func (h *UserHandler) handleErrorWithSession(w http.ResponseWriter, r *http.Request, status int, msg string) {
	h.refreshSessionIfExists(w, r)
	h.handleError(w, status, msg)
}

// Вспомогательный метод для парсинга ID из URL /events/{id} или /users/{id}
func (h *UserHandler) extractID(r *http.Request, prefix string) string {
	return strings.TrimPrefix(r.URL.Path, prefix)
}

// --- Обработчики Пользователей ---

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
		h.handleError(w, http.StatusUnauthorized, "invalid credentials")
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

func (h *UserHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("X-Session-Id")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	_ = h.sessionRepo.DeleteSession(r.Context(), cookie.Value)
	h.setSessionCookie(w, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

// GET /users - поиск организаторов
func (h *UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.ParseInt(q.Get("limit"), 10, 64)
	offset, _ := strconv.ParseInt(q.Get("offset"), 10, 64)
	if limit <= 0 {
		limit = 10
	}

	users, count, err := h.userService.FindUsers(r.Context(), q.Get("name"), q.Get("id"), limit, offset)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	h.refreshSessionIfExists(w, r)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"users": users, "count": count})
}

// GET /users/{id} - карточка организатора
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	id := h.extractID(r, "/users/")
	user, err := h.userService.GetUserByID(r.Context(), id)
	if err != nil {
		h.handleError(w, http.StatusNotFound, "Not found")
		return
	}
	h.refreshSessionIfExists(w, r)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// GET /users/{id}/events - события конкретного организатора
func (h *UserHandler) GetUserEvents(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(h.extractID(r, "/users/"), "/events")

	// Проверяем существование пользователя
	if _, err := h.userService.GetUserByID(r.Context(), id); err != nil {
		h.handleError(w, http.StatusNotFound, "User not found")
		return
	}

	filters := map[string]interface{}{"created_by": id}
	events, count, err := h.userService.ListEvents(r.Context(), filters, 100, 0)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	h.refreshSessionIfExists(w, r)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"events": events, "count": count})
}

// --- Обработчики Событий ---

func (h *UserHandler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("X-Session-Id")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	uid, _ := h.sessionService.GetUserID(r.Context(), cookie.Value)
	if uid == "" {
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
		status := http.StatusInternalServerError
		if mongo.IsDuplicateKeyError(err) {
			status = http.StatusConflict
		}
		w.WriteHeader(status)
		return
	}

	h.refreshSessionIfExists(w, r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// GET /events - расширенный поиск
func (h *UserHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filters := map[string]interface{}{
		"title":     q.Get("title"),
		"id":        q.Get("id"),
		"category":  q.Get("category"),
		"city":      q.Get("city"),
		"user":      q.Get("user"),
		"date_from": q.Get("date_from"),
		"date_to":   q.Get("date_to"),
	}

	if pf := q.Get("price_from"); pf != "" {
		v, _ := strconv.ParseUint(pf, 10, 32)
		filters["price_from"] = uint(v)
	}
	if pt := q.Get("price_to"); pt != "" {
		v, _ := strconv.ParseUint(pt, 10, 32)
		filters["price_to"] = uint(v)
	}

	limit, _ := strconv.ParseInt(q.Get("limit"), 10, 64)
	offset, _ := strconv.ParseInt(q.Get("offset"), 10, 64)
	if limit <= 0 {
		limit = 10
	}

	events, count, err := h.userService.ListEvents(r.Context(), filters, limit, offset)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	h.refreshSessionIfExists(w, r)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"events": events, "count": count})
}

// GET /events/{id} - карточка мероприятия
func (h *UserHandler) GetEvent(w http.ResponseWriter, r *http.Request) {
	id := h.extractID(r, "/events/")
	event, err := h.userService.GetEventByID(r.Context(), id)
	if err != nil {
		h.handleError(w, http.StatusNotFound, "Not found")
		return
	}
	h.refreshSessionIfExists(w, r)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(event)
}

// PATCH /events/{id} - редактирование
func (h *UserHandler) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	eventID := h.extractID(r, "/events/")
	cookie, err := r.Cookie("X-Session-Id")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	uid, _ := h.sessionService.GetUserID(r.Context(), cookie.Value)
	if uid == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var req struct {
		Category *string `json:"category"`
		Price    *uint   `json:"price"`
		City     *string `json:"city"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	updates := bson.M{}
	if req.Category != nil {
		valid := map[string]bool{"meetup": true, "concert": true, "exhibition": true, "party": true, "other": true}
		if !valid[*req.Category] {
			h.handleError(w, http.StatusBadRequest, "invalid \"category\" field")
			return
		}
		updates["category"] = *req.Category
	}
	if req.Price != nil {
		updates["price"] = *req.Price
	}
	if req.City != nil {
		updates["location.city"] = *req.City
	}

	found, err := h.userService.PatchEvent(r.Context(), eventID, uid, updates)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !found {
		h.handleError(w, http.StatusNotFound, "Not found. Be sure that event exists and you are the organizer")
		return
	}

	h.setSessionCookie(w, cookie.Value, h.sessionTTL)
	w.WriteHeader(http.StatusNoContent)
}
