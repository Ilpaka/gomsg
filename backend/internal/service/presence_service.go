package service

import (
	"context"

	"goflow/backend/internal/domain"
	redistore "goflow/backend/internal/repository/redis"
)

// PresenceService exposes online state backed by Redis TTL keys.
type PresenceService struct {
	repo *redistore.PresenceRepository
}

func NewPresenceService(repo *redistore.PresenceRepository) *PresenceService {
	return &PresenceService{repo: repo}
}

// SetOnline marks the user online and starts/refreshes heartbeat TTL.
func (s *PresenceService) SetOnline(ctx context.Context, userID domain.ID) error {
	if s == nil || s.repo == nil {
		return nil
	}
	return s.repo.SetOnline(ctx, userID)
}

// SetOffline removes online state.
func (s *PresenceService) SetOffline(ctx context.Context, userID domain.ID) error {
	if s == nil || s.repo == nil {
		return nil
	}
	return s.repo.SetOffline(ctx, userID)
}

// IsOnline reports whether the user is currently within the presence TTL window.
func (s *PresenceService) IsOnline(ctx context.Context, userID domain.ID) (bool, error) {
	if s == nil || s.repo == nil {
		return false, nil
	}
	return s.repo.IsOnline(ctx, userID)
}

// Touch refreshes heartbeat TTL (same as SetOnline).
func (s *PresenceService) Touch(ctx context.Context, userID domain.ID) error {
	return s.SetOnline(ctx, userID)
}

// Connected implements ws.PresenceNotifier.
func (s *PresenceService) Connected(ctx context.Context, userID domain.ID) {
	_ = s.SetOnline(ctx, userID)
}

// Disconnected implements ws.PresenceNotifier.
func (s *PresenceService) Disconnected(ctx context.Context, userID domain.ID) {
	_ = s.SetOffline(ctx, userID)
}

// Activity implements ws.PresenceNotifier.
func (s *PresenceService) Activity(ctx context.Context, userID domain.ID) {
	_ = s.Touch(ctx, userID)
}
