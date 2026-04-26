package service

import (
	"context"
	"time"

	apperr "goflow/backend/internal/pkg/errors"
	"goflow/backend/internal/domain"
)

// WSTicketIssuer abstracts Redis-backed WS tickets (implemented by redis.WSTicketStore).
type WSTicketIssuer interface {
	Issue(ctx context.Context, userID domain.ID, ttl time.Duration) (ticket string, err error)
}

// WSTicketService creates short-lived tickets for WebSocket upgrade.
type WSTicketService struct {
	redis WSTicketIssuer
	ttl   time.Duration
}

func NewWSTicketService(r WSTicketIssuer, ttl time.Duration) *WSTicketService {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return &WSTicketService{redis: r, ttl: ttl}
}

func (s *WSTicketService) Issue(ctx context.Context, userID domain.ID) (ticket string, expiresInSec int, err error) {
	if s == nil || s.redis == nil {
		return "", 0, apperr.Internal("ws ticket store not configured", nil)
	}
	if userID == "" {
		return "", 0, apperr.Unauthorized("missing user")
	}
	tok, err := s.redis.Issue(ctx, userID, s.ttl)
	if err != nil {
		return "", 0, apperr.Internal("issue ws ticket", err)
	}
	return tok, int(s.ttl.Seconds()), nil
}
