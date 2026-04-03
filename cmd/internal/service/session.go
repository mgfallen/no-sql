package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// SessionRepo - обновленный интерфейс хранилища (Redis)
type SessionRepo interface {
	CreateSession(ctx context.Context, sid string) (bool, error)
	RefreshTTL(ctx context.Context, sid string) error
	Exists(ctx context.Context, sid string) (bool, error)
	GetUserIDBySession(ctx context.Context, sid string) (string, error)
}

type SessionService struct {
	repo SessionRepo
}

func NewSessionService(repo SessionRepo) *SessionService {
	return &SessionService{repo: repo}
}

func (s *SessionService) GenerateSID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *SessionService) CheckExists(ctx context.Context, sid string) (bool, error) {
	if sid == "" {
		return false, nil
	}
	return s.repo.Exists(ctx, sid)
}

// GetUserID реализует метод интерфейса SessionManager из хендлера
func (s *SessionService) GetUserID(ctx context.Context, sid string) (string, error) {
	if sid == "" {
		return "", nil
	}
	return s.repo.GetUserIDBySession(ctx, sid)
}

// HandleSessionRequest оставляем для совместимости с прошлыми лабами
func (s *SessionService) HandleSessionRequest(ctx context.Context, existingSid string) (string, bool, error) {
	if existingSid != "" {
		exists, err := s.repo.Exists(ctx, existingSid)
		if err == nil && exists {
			_ = s.repo.RefreshTTL(ctx, existingSid)
			return existingSid, false, nil
		}
	}

	newSid, err := s.GenerateSID()
	if err != nil {
		return "", false, err
	}

	_, err = s.repo.CreateSession(ctx, newSid)
	return newSid, true, err
}
