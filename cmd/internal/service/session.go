package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// SessionRepo - интерфейс хранилища (Redis)
type SessionRepo interface {
	CreateSession(ctx context.Context, sid string) (bool, error)
	RefreshTTL(ctx context.Context, sid string) error
	Exists(ctx context.Context, sid string) (bool, error)
	GetUserIDBySession(ctx context.Context, sid string) (string, error)
	BindUser(ctx context.Context, sid string, userID string) error
	DeleteSession(ctx context.Context, sid string) error
}

type SessionService struct {
	repo SessionRepo
}

// NewSessionService - конструктор сервиса сессий
func NewSessionService(repo SessionRepo) *SessionService {
	return &SessionService{repo: repo}
}

// GenerateSID генерирует криптографически стойкий случайный идентификатор сессии
func (s *SessionService) GenerateSID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes for sid: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// CheckExists проверяет существование сессии в Redis с пробросом ошибок
func (s *SessionService) CheckExists(ctx context.Context, sid string) (bool, error) {
	if sid == "" {
		return false, nil
	}
	return s.repo.Exists(ctx, sid)
}

// GetUserID возвращает ID пользователя, привязанного к сессии
func (s *SessionService) GetUserID(ctx context.Context, sid string) (string, error) {
	if sid == "" {
		return "", nil
	}
	return s.repo.GetUserIDBySession(ctx, sid)
}

// BindUser связывает сессию с пользователем при Login/Register
func (s *SessionService) BindUser(ctx context.Context, sid string, userID string) error {
	if sid == "" || userID == "" {
		return fmt.Errorf("sid or userID cannot be empty")
	}
	return s.repo.BindUser(ctx, sid, userID)
}

// CreateSession - создание сессии в Redis
func (s *SessionService) CreateSession(ctx context.Context, sid string) (bool, error) {
	if sid == "" {
		return false, fmt.Errorf("sid cannot be empty")
	}
	return s.repo.CreateSession(ctx, sid)
}

// RefreshTTL - продление времени жизни сессии
func (s *SessionService) RefreshTTL(ctx context.Context, sid string) error {
	if sid == "" {
		return nil
	}
	return s.repo.RefreshTTL(ctx, sid)
}

// DeleteSession - удаление сессии из Redis (Logout)
func (s *SessionService) DeleteSession(ctx context.Context, sid string) error {
	if sid == "" {
		return nil
	}
	return s.repo.DeleteSession(ctx, sid)
}

// HandleSessionRequest обрабатывает логику создания или продления сессии
// Теперь ошибки Redis обрабатываются корректно и возвращаются наверх (в хендлер)
func (s *SessionService) HandleSessionRequest(ctx context.Context, existingSid string) (string, bool, error) {
	if existingSid != "" {
		exists, err := s.repo.Exists(ctx, existingSid)
		if err != nil {
			// Если Redis упал — возвращаем ошибку, а не маскируем её созданием новой сессии
			return "", false, fmt.Errorf("failed to check existing session: %w", err)
		}
		if exists {
			if err := s.repo.RefreshTTL(ctx, existingSid); err != nil {
				return "", false, fmt.Errorf("failed to refresh session ttl: %w", err)
			}
			return existingSid, false, nil
		}
	}

	// Если сессии не было или она протухла в Redis — генерируем новую
	newSid, err := s.GenerateSID()
	if err != nil {
		return "", false, err
	}

	_, err = s.repo.CreateSession(ctx, newSid)
	if err != nil {
		return "", false, fmt.Errorf("failed to save new session to redis: %w", err)
	}

	return newSid, true, nil
}
