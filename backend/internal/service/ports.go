// Package service holds the application's use cases. It depends only on the
// domain and on the interfaces below; storage and transport depend on it,
// never the other way round.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/hyperheaven04/go_chat/backend/internal/domain"
)

type UserRepository interface {
	Create(ctx context.Context, username, passwordHash string) (domain.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (domain.User, error)
	GetCredentials(ctx context.Context, username string) (domain.User, string, error)
	Search(ctx context.Context, prefix string, exclude uuid.UUID, limit int) ([]domain.User, error)
	TouchLastSeen(ctx context.Context, id uuid.UUID, at time.Time) error
}

type DialogRepository interface {
	GetOrCreate(ctx context.Context, u1, u2 uuid.UUID) (domain.Dialog, bool, error)
	Get(ctx context.Context, id uuid.UUID) (domain.Dialog, error)
	PeerIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	ListSummaries(ctx context.Context, userID uuid.UUID) ([]domain.DialogSummary, error)
	GetSummary(ctx context.Context, userID, dialogID uuid.UUID) (domain.DialogSummary, error)
}

type MessageRepository interface {
	Create(ctx context.Context, in domain.NewMessage) (domain.Message, bool, error)
	List(ctx context.Context, dialogID uuid.UUID, beforeID int64, limit int) ([]domain.Message, error)
	MarkRead(ctx context.Context, dialogID, userID uuid.UUID, messageID int64) (bool, error)
}

type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) bool
	CompareDummy(password string)
}

type TokenIssuer interface {
	Issue(userID uuid.UUID) (string, time.Time, error)
}

// Notifier delivers real-time events to users' live connections.
// Delivery is best effort: persistent state is always in the database and
// clients resync via REST after reconnecting.
type Notifier interface {
	Publish(recipients []uuid.UUID, ev domain.Event)
}

type PresenceTracker interface {
	IsOnline(userID uuid.UUID) bool
}
