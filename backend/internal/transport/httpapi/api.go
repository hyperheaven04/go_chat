// Package httpapi implements the REST API.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"github.com/hyperheaven04/go_chat/backend/internal/domain"
	"github.com/hyperheaven04/go_chat/backend/internal/service"
)

type AuthUseCase interface {
	Register(ctx context.Context, username, password string) (service.AuthResult, error)
	Login(ctx context.Context, username, password string) (service.AuthResult, error)
}

type ChatUseCase interface {
	Me(ctx context.Context, userID uuid.UUID) (domain.User, error)
	IsOnline(userID uuid.UUID) bool
	SearchUsers(ctx context.Context, userID uuid.UUID, query string) ([]domain.User, error)
	OpenDialog(ctx context.Context, userID, peerID uuid.UUID) (domain.DialogSummary, bool, error)
	ListDialogs(ctx context.Context, userID uuid.UUID) ([]domain.DialogSummary, error)
	GetDialog(ctx context.Context, userID, dialogID uuid.UUID) (domain.DialogSummary, error)
	ListMessages(ctx context.Context, userID, dialogID uuid.UUID, beforeID int64, limit int) ([]domain.Message, error)
}

type TokenParser interface {
	Parse(token string) (uuid.UUID, error)
}

type TicketIssuer interface {
	Issue(userID uuid.UUID) (string, time.Duration)
}

type Pinger interface {
	Ping(ctx context.Context) error
}

type Deps struct {
	Auth           AuthUseCase
	Chat           ChatUseCase
	Tokens         TokenParser
	Tickets        TicketIssuer
	DB             Pinger
	WebSocket      http.Handler
	AllowedOrigins []string
	Log            *slog.Logger
}

type API struct {
	Deps
	authLimiter *ipRateLimiter
}

func New(d Deps) *API {
	return &API{
		Deps: d,
		// Brute-force protection for login/registration: 5 requests/min per IP, burst 10.
		authLimiter: newIPRateLimiter(rate.Every(12*time.Second), 10),
	}
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", a.healthz)
	mux.HandleFunc("GET /readyz", a.readyz)

	mux.Handle("POST /api/v1/auth/register", a.authLimiter.middleware(http.HandlerFunc(a.register)))
	mux.Handle("POST /api/v1/auth/login", a.authLimiter.middleware(http.HandlerFunc(a.login)))

	mux.Handle("GET /api/v1/users/me", a.authenticate(a.me))
	mux.Handle("GET /api/v1/users", a.authenticate(a.searchUsers))
	mux.Handle("GET /api/v1/dialogs", a.authenticate(a.listDialogs))
	mux.Handle("POST /api/v1/dialogs", a.authenticate(a.openDialog))
	mux.Handle("GET /api/v1/dialogs/{id}", a.authenticate(a.getDialog))
	mux.Handle("GET /api/v1/dialogs/{id}/messages", a.authenticate(a.listMessages))
	mux.Handle("POST /api/v1/ws/ticket", a.authenticate(a.issueTicket))
	mux.Handle("GET /api/v1/ws", a.WebSocket) // authenticated by ticket inside

	// Outermost first.
	return chain(mux, a.requestID, a.accessLog, a.recoverer, securityHeaders, a.cors)
}
