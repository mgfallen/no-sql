package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"no-sql/cmd/internal/domain"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// UserProcessor описывает бизнес-логику пользователей и событий
type UserProcessor interface {
	Register(ctx context.Context, fullName, username, password string) (*domain.User, error)
	Login(ctx context.Context, username, password string) (*domain.User, error)
	CreateEvent(ctx context.Context, event *domain.Event) (string, error)
	ListEvents(ctx context.Context, filter map[string]string, limit, offset int64) ([]domain.Event, int64, error)
	UpdateEvent(ctx context.Context, id string, createdBy string, category string, price *uint64, city *string) error
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

// Функция-хелпер для проверки дубликатов в MongoDB Go Driver v2
func isDuplicateKeyErr(err error) bool {
	var wExc mongo.WriteException
	if errors.As(err, &wExc) {
		for _, we := range wExc.WriteErrors {
			if we.Code == 11000 {
				return true
			}
		}
	}
	var cErr mongo.CommandError
	if errors.As(err, &cErr) {
		return cErr.Code == 11000
	}
	return false
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
		h.handleErrorWithSession(w, r, http.StatusBadRequest, "invalid json body")
		return
	}

	if req.FullName == "" || req.Username == "" || req.Password == "" {
		h.handleErrorWithSession(w, r, http.StatusBadRequest, "required fields missing")
		return
	}

	user, err := h.userService.Register(r.Context(), req.FullName, req.Username, req.Password)
	if err != nil {
		if isDuplicateKeyErr(err) {
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
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "invalid credentials"})
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
		h.handleErrorWithSession(w, r, http.StatusBadRequest, "invalid json body")
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
		if isDuplicateKeyErr(err) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "event already exists"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_ = h.sessionRepo.RefreshTTL(r.Context(), cookie.Value)
	h.setSessionCookie(w, cookie.Value, h.sessionTTL)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// ListEvents - GET /events
func (h *UserHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filters := make(map[string]string)
	if q.Get("title") != "" {
		filters["title"] = q.Get("title")
	}
	if q.Get("id") != "" {
		filters["id"] = q.Get("id")
	}
	if q.Get("category") != "" {
		filters["category"] = q.Get("category")
	}
	if q.Get("price_from") != "" {
		filters["price_from"] = q.Get("price_from")
	}
	if q.Get("price_to") != "" {
		filters["price_to"] = q.Get("price_to")
	}
	if q.Get("city") != "" {
		filters["city"] = q.Get("city")
	}
	if q.Get("started_date_from") != "" {
		filters["started_date_from"] = q.Get("started_date_from")
	}
	if q.Get("started_date_to") != "" {
		filters["started_date_to"] = q.Get("started_date_to")
	}
	if q.Get("user") != "" {
		filters["user"] = q.Get("user")
	}

	limit, _ := strconv.ParseInt(q.Get("limit"), 10, 64)
	offset, _ := strconv.ParseInt(q.Get("offset"), 10, 64)

	if limit == 0 {
		limit = 10
	}

	events, count, err := h.userService.ListEvents(r.Context(), filters, limit, offset)
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
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"events": events,
		"count":  count,
	})
}

// UpdateEvent - PATCH /events/{id}
func (h *UserHandler) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

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

	// Извлекаем ID из URL пути независимо от наличия базовых префиксов роутера
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	var eventID string
	if len(pathParts) > 0 {
		eventID = pathParts[len(pathParts)-1]
	}

	if eventID == "" || eventID == "events" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "invalid \"id\" field"})
		return
	}

	var req struct {
		Category string  `json:"category"`
		Price    *uint64 `json:"price"`
		City     *string `json:"city"`
	}

	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "invalid json body"})
		return
	}

	// Валидация категорий строго по спецификации ЛР-4
	if req.Category != "" {
		validCategories := map[string]bool{
			"meetup":     true,
			"concert":    true,
			"exhibition": true,
			"party":      true,
			"other":      true,
		}
		if !validCategories[req.Category] {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "invalid \"category\" field"})
			return
		}
	}

	// Шаг 1: Проверяем существование события через ListEvents (фильтруя по id)
	events, _, err := h.userService.ListEvents(r.Context(), map[string]string{"id": eventID}, 1, 0)
	if err != nil || len(events) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "event not found"})
		return
	}

	// Шаг 2: Проверяем права владения (Текущий юзер должен быть создателем)
	if events[0].CreatedBy != uid {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden) // Возвращаем 403 Forbidden для чужих событий по ТЗ ЛР-4
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "you are not the organizer of this event"})
		return
	}

	// Шаг 3: Вызываем обновление в БД
	err = h.userService.UpdateEvent(r.Context(), eventID, uid, req.Category, req.Price, req.City)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "event not found"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Шаг 4: Продлеваем сессию и устанавливаем обязательную куку ответа
	_ = h.sessionRepo.RefreshTTL(r.Context(), cookie.Value)
	h.setSessionCookie(w, cookie.Value, h.sessionTTL)
	w.WriteHeader(http.StatusNoContent)
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
	_ = json.NewEncoder(w).Encode(map[string]string{"message": msg})
}
