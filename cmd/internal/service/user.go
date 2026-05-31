package service

import (
	"context"
	"errors"
	"time"

	"no-sql/cmd/internal/domain"

	"golang.org/x/crypto/bcrypt"
)

// UserRepo описывает контракты для работы с MongoDB
type UserRepo interface {
	CreateUser(ctx context.Context, user *domain.User) error
	GetUserByUsername(ctx context.Context, username string) (*domain.User, error)
	CreateEvent(ctx context.Context, event *domain.Event) (string, error)
	QueryEvents(ctx context.Context, filters map[string]string, limit, offset int64) ([]domain.Event, int64, error)
	UpdateEvent(ctx context.Context, id string, createdBy string, category string, price *uint64, city *string) error
}

type UserService struct {
	repo UserRepo
}

func NewUserService(repo UserRepo) *UserService {
	return &UserService{repo: repo}
}

func (s *UserService) Register(ctx context.Context, fullName, username, password string) (*domain.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		FullName:     fullName,
		Username:     username,
		PasswordHash: string(hash),
	}

	err = s.repo.CreateUser(ctx, user)
	return user, err
}

func (s *UserService) Login(ctx context.Context, username, password string) (*domain.User, error) {
	user, err := s.repo.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, errors.New("invalid credentials")
	}

	return user, nil
}

// CreateEvent реализует создание события через репозиторий
func (s *UserService) CreateEvent(ctx context.Context, event *domain.Event) (string, error) {
	event.CreatedAt = time.Now().Format(time.RFC3339)
	return s.repo.CreateEvent(ctx, event)
}

// ListEvents теперь принимает map[string]string и делегирует построение bson.M репозиторию
func (s *UserService) ListEvents(ctx context.Context, filters map[string]string, limit, offset int64) ([]domain.Event, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.QueryEvents(ctx, filters, limit, offset)
}

// UpdateEvent выполняет PATCH-обновление эвента с проверкой прав создателя
func (s *UserService) UpdateEvent(ctx context.Context, id string, createdBy string, category string, price *uint64, city *string) error {
	if id == "" || createdBy == "" {
		return errors.New("invalid event id or creator id")
	}
	return s.repo.UpdateEvent(ctx, id, createdBy, category, price, city)
}
