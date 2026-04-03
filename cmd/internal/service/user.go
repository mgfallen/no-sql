package service

import (
	"context"
	"errors"
	"no-sql/cmd/internal/domain"

	"golang.org/x/crypto/bcrypt"
)

// UserRepo - интерфейс репозитория
type UserRepo interface {
	CreateUser(ctx context.Context, user *domain.User) error
	GetUserByUsername(ctx context.Context, username string) (*domain.User, error)
}

// UserService - сервис для юзеров
type UserService struct {
	repo UserRepo
}

// NewUserService - конструктор
func NewUserService(repo UserRepo) *UserService {
	return &UserService{repo: repo}
}

// Register - регистрация
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

// Login - войти
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
