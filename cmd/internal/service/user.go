package service

import (
	"context"
	"errors"
	"no-sql/cmd/internal/domain"

	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/crypto/bcrypt"
)

// UserRepo описывает методы работы с коллекцией пользователей
type UserRepo interface {
	CreateUser(ctx context.Context, user *domain.User) error
	GetUserByUsername(ctx context.Context, username string) (*domain.User, error)
	GetUserByID(ctx context.Context, id string) (*domain.User, error)
	SearchUsers(ctx context.Context, name, id string, limit, offset int64) ([]domain.User, int64, error)
}

// EventRepo описывает методы работы с коллекцией событий
type EventRepo interface {
	CreateEvent(ctx context.Context, event *domain.Event) (string, error)
	FindEvents(ctx context.Context, filters map[string]interface{}, limit, offset int64) ([]domain.Event, int64, error)
	GetEventByID(ctx context.Context, id string) (*domain.Event, error)
	UpdateEvent(ctx context.Context, eid, uid string, updates bson.M) (bool, error)
}

type UserService struct {
	userRepo  UserRepo
	eventRepo EventRepo
}

func NewUserService(ur UserRepo, er EventRepo) *UserService {
	return &UserService{
		userRepo:  ur,
		eventRepo: er,
	}
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

	err = s.userRepo.CreateUser(ctx, user)
	return user, err
}

func (s *UserService) Login(ctx context.Context, username, password string) (*domain.User, error) {
	user, err := s.userRepo.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, errors.New("invalid credentials")
	}

	return user, nil
}

func (s *UserService) FindUsers(ctx context.Context, name, id string, limit, offset int64) ([]domain.User, int64, error) {
	return s.userRepo.SearchUsers(ctx, name, id, limit, offset)
}

func (s *UserService) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	return s.userRepo.GetUserByID(ctx, id)
}

// --- Event Logic ---

func (s *UserService) CreateEvent(ctx context.Context, event *domain.Event) (string, error) {
	// Дефолтная категория, если не указана
	if event.Category == "" {
		event.Category = "other"
	}
	return s.eventRepo.CreateEvent(ctx, event)
}

func (s *UserService) ListEvents(ctx context.Context, filters map[string]interface{}, limit, offset int64) ([]domain.Event, int64, error) {
	return s.eventRepo.FindEvents(ctx, filters, limit, offset)
}

func (s *UserService) GetEventByID(ctx context.Context, id string) (*domain.Event, error) {
	return s.eventRepo.GetEventByID(ctx, id)
}

func (s *UserService) PatchEvent(ctx context.Context, eid, uid string, updates bson.M) (bool, error) {
	return s.eventRepo.UpdateEvent(ctx, eid, uid, updates)
}
