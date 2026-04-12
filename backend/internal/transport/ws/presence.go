package ws

import (
	"context"

	"goflow/backend/internal/domain"
)

// PresenceNotifier updates ephemeral presence for websocket sessions (optional).
type PresenceNotifier interface {
	Connected(ctx context.Context, userID domain.ID)
	Disconnected(ctx context.Context, userID domain.ID)
	Activity(ctx context.Context, userID domain.ID)
}
