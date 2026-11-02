package ws

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	"github.com/hyperheaven04/go_chat/backend/internal/domain"
)

var errHubClosed = errors.New("hub is shutting down")

// Hub tracks live connections per user and fans events out to them.
//
// A user may have several connections (phone + desktop); all of them receive
// events. The registry is guarded by an RWMutex: publishing only reads it and
// never blocks on a slow client, because each client has its own buffered
// queue (see Client.enqueue).
//
// The hub is in-process, so it serves a single instance. Scaling out would
// put a pub/sub bus (Redis, NATS) behind Publish without touching the service layer.
type Hub struct {
	mu      sync.RWMutex
	clients map[uuid.UUID]map[*Client]struct{}
	closed  bool
	wg      sync.WaitGroup // live connection handlers

	ctx    context.Context // parent of every client context; cancelled on shutdown
	cancel context.CancelFunc
	log    *slog.Logger
}

func NewHub(log *slog.Logger) *Hub {
	ctx, cancel := context.WithCancel(context.Background())
	return &Hub{clients: make(map[uuid.UUID]map[*Client]struct{}), ctx: ctx, cancel: cancel, log: log}
}

// register adds the client and reports whether it is the user's first connection.
// On success the caller must call done once it has fully finished with the connection.
func (h *Hub) register(c *Client) (first bool, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return false, errHubClosed
	}
	h.wg.Add(1)
	set, ok := h.clients[c.userID]
	if !ok {
		set = make(map[*Client]struct{})
		h.clients[c.userID] = set
	}
	set[c] = struct{}{}
	return len(set) == 1, nil
}

// unregister removes the client and reports whether it was the user's last connection.
func (h *Hub) unregister(c *Client) (last bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.clients[c.userID]
	delete(set, c)
	if len(set) == 0 {
		delete(h.clients, c.userID)
		return true
	}
	return false
}

func (h *Hub) done() { h.wg.Done() }

func (h *Hub) IsOnline(userID uuid.UUID) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID]) > 0
}

// Publish implements service.Notifier. The event is encoded once and the
// same bytes are queued to every connection of every recipient.
func (h *Hub) Publish(recipients []uuid.UUID, ev domain.Event) {
	b, err := encodeEvent(ev)
	if err != nil {
		h.log.Error("encode event", "error", err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, id := range recipients {
		for c := range h.clients[id] {
			c.enqueue(b)
		}
	}
}

// Shutdown closes all connections with "going away" and waits for their
// handlers to finish, or for ctx to expire.
func (h *Hub) Shutdown(ctx context.Context) error {
	h.mu.Lock()
	h.closed = true
	h.mu.Unlock()
	h.cancel()

	done := make(chan struct{})
	go func() { h.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
