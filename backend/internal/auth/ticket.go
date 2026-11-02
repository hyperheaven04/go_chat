package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TicketStore issues short-lived, single-use tickets for opening a WebSocket.
//
// Browsers cannot set an Authorization header on a WebSocket handshake, and
// putting the long-lived JWT into the URL would leak it into proxy and access
// logs. Instead the client exchanges its JWT for a ticket over an
// authenticated REST call and passes the ticket in the URL. A leaked ticket
// is useless: it expires in seconds and works only once.
//
// The store is in-memory, which is fine for a single instance; with several
// replicas it would move to Redis.
type TicketStore struct {
	mu    sync.Mutex
	ttl   time.Duration
	items map[string]ticket
	now   func() time.Time
}

type ticket struct {
	userID    uuid.UUID
	expiresAt time.Time
}

func NewTicketStore(ttl time.Duration) *TicketStore {
	return &TicketStore{ttl: ttl, items: make(map[string]ticket), now: time.Now}
}

func (s *TicketStore) Issue(userID uuid.UUID) (string, time.Duration) {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error
	t := base64.RawURLEncoding.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	// Lazy sweep keeps the map bounded without a background goroutine.
	for k, v := range s.items {
		if now.After(v.expiresAt) {
			delete(s.items, k)
		}
	}
	s.items[t] = ticket{userID: userID, expiresAt: now.Add(s.ttl)}
	return t, s.ttl
}

// Consume validates the ticket and invalidates it.
func (s *TicketStore) Consume(t string) (uuid.UUID, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.items[t]
	if !ok {
		return uuid.Nil, false
	}
	delete(s.items, t)
	if s.now().After(v.expiresAt) {
		return uuid.Nil, false
	}
	return v.userID, true
}
