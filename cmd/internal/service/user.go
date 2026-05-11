package service

import (
	"context"
	"errors"
	"no-sql/cmd/internal/domain"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// UserRepo -
type UserRepo interface {
	CreateUser(ctx context.Context, user *domain.User) error
	GetUserByUsername(ctx context.Context, username string) (*domain.User, error)
	CreateEvent(ctx context.Context, event *domain.Event) (string, error)
	GetEvents(ctx context.Context, title string, limit, offset int64) ([]domain.Event, int64, error)
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
	// Проставляем системную дату создания перед сохранением
	event.CreatedAt = time.Now().Format(time.RFC3339)
	return s.repo.CreateEvent(ctx, event)
}

// ListEvents реализует получение списка с фильтрацией
func (s *UserService) ListEvents(ctx context.Context, title string, limit, offset int64) ([]domain.Event, int64, error) {
	return s.repo.GetEvents(ctx, title, limit, offset)
}
