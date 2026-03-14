package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// SessionRepo - интерфейс хранилища
type SessionRepo interface {
	CreateSession(ctx context.Context, sid string) (bool, error)
	RefreshTTL(ctx context.Context, sid string) error
	Exists(ctx context.Context, sid string) (bool, error)
}

// SessionService - сервис для сессии
type SessionService struct {
	repo SessionRepo
}

// NewSessionService - конструктор
func NewSessionService(repo SessionRepo) *SessionService {
	return &SessionService{
		repo: repo,
	}
}

func (s *SessionService) GenerateSID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HandleSessionRequest -
func (s *SessionService) HandleSessionRequest(ctx context.Context, existingSid string) (string, bool, error) {
	if existingSid != "" {
		exists, err := s.repo.Exists(ctx, existingSid)
		if err == nil && exists {
			err = s.repo.RefreshTTL(ctx, existingSid)
			if err != nil {
				return "", false, err
			}
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

// CheckExists -
func (s *SessionService) CheckExists(ctx context.Context, sid string) (bool, error) {
	if sid == "" {
		return false, nil
	}
	return s.repo.Exists(ctx, sid)
}
