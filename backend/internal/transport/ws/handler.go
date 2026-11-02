package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/hyperheaven04/go_chat/backend/internal/transport/dto"
)

type TicketConsumer interface {
	Consume(ticket string) (uuid.UUID, bool)
}

const presenceTimeout = 5 * time.Second

// Handler upgrades authenticated requests to WebSocket connections.
type Handler struct {
	hub            *Hub
	chat           ChatAPI
	tickets        TicketConsumer
	originPatterns []string
	log            *slog.Logger
}

// NewHandler: originPatterns are hosts (e.g. "localhost:3000") allowed to open
// connections from a browser; native clients send no Origin and are allowed.
func NewHandler(hub *Hub, chat ChatAPI, tickets TicketConsumer, originPatterns []string, log *slog.Logger) *Handler {
	return &Handler{hub: hub, chat: chat, tickets: tickets, originPatterns: originPatterns, log: log}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Authenticate before upgrading, so unauthenticated clients never get a socket.
	userID, ok := h.tickets.Consume(r.URL.Query().Get("ticket"))
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": dto.Error{Code: "unauthorized", Message: "invalid or expired ticket"},
		})
		return
	}

	// The http.Server read/write timeouts would otherwise kill a long-lived socket.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})

	// Accept rejects cross-site browser requests (CSWSH) unless the Origin is allowed.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.originPatterns})
	if err != nil {
		h.log.Debug("websocket accept", "error", err)
		return
	}
	conn.SetReadLimit(maxFrameSize)

	c := newClient(h.hub.ctx, userID, conn, h.chat, h.log)
	first, err := h.hub.register(c)
	if err != nil {
		_ = conn.Close(websocket.StatusTryAgainLater, "server shutting down")
		return
	}
	defer h.hub.done() // after UserDisconnected, so shutdown waits for it
	// Presence work must outlive the request context.
	bg := context.WithoutCancel(r.Context())
	if first {
		ctx, cancel := context.WithTimeout(bg, presenceTimeout)
		h.chat.UserConnected(ctx, userID)
		cancel()
	}

	c.run()

	if h.hub.unregister(c) {
		ctx, cancel := context.WithTimeout(bg, presenceTimeout)
		h.chat.UserDisconnected(ctx, userID)
		cancel()
	}
}
