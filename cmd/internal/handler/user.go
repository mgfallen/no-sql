package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"no-sql/cmd/internal/domain"

	"go.mongodb.org/mongo-driver/mongo"
)

// UserProcessor - интерфейс методов работы с пользователями
type UserProcessor interface {
	CreateUser(ctx context.Context, user *domain.User) error
	GetUserByUsername(ctx context.Context, username string) (*domain.User, error)
}

// UserHandler - обрабатывает HTTP запросы, связанные с пользователями
type UserHandler struct {
	service UserProcessor
}

// NewUserHandler - конструктор
func NewUserHandler(svc UserProcessor) *UserHandler {
	return &UserHandler{
		service: svc,
	}
}

// Register - создание нового пользователя (POST /user/register)
func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var user domain.User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if user.Username == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err := h.service.CreateUser(r.Context(), &user)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// GetProfile - получение данных текущего пользователя (GET /user/profile)
func (h *UserHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie("X-Session-Id")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	user, err := h.service.GetUserByUsername(r.Context(), cookie.Value)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}
