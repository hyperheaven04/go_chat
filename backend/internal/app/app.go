// Package app wires the application's dependencies together.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hyperheaven04/go_chat/backend/internal/auth"
	"github.com/hyperheaven04/go_chat/backend/internal/config"
	"github.com/hyperheaven04/go_chat/backend/internal/service"
	"github.com/hyperheaven04/go_chat/backend/internal/storage/postgres"
	"github.com/hyperheaven04/go_chat/backend/internal/transport/httpapi"
	"github.com/hyperheaven04/go_chat/backend/internal/transport/ws"
)

type App struct {
	Handler http.Handler
	pool    *pgxpool.Pool
	hub     *ws.Hub
}

func New(ctx context.Context, cfg config.Config, log *slog.Logger) (*App, error) {
	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if err := postgres.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	hasher, err := auth.NewPasswordHasher(cfg.BcryptCost)
	if err != nil {
		pool.Close()
		return nil, err
	}
	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTTTL)
	tickets := auth.NewTicketStore(cfg.WSTicketTTL)

	users := postgres.NewUserRepo(pool)
	hub := ws.NewHub(log)

	authSvc := service.NewAuthService(users, hasher, tokens)
	chatSvc := service.NewChatService(users, postgres.NewDialogRepo(pool), postgres.NewMessageRepo(pool), hub, hub, log)

	api := httpapi.New(httpapi.Deps{
		Auth:           authSvc,
		Chat:           chatSvc,
		Tokens:         tokens,
		Tickets:        tickets,
		DB:             pool,
		WebSocket:      ws.NewHandler(hub, chatSvc, tickets, originHosts(cfg.AllowedOrigins), log),
		AllowedOrigins: cfg.AllowedOrigins,
		Log:            log,
	})

	return &App{Handler: api.Handler(), pool: pool, hub: hub}, nil
}

// Shutdown closes WebSocket connections, then the database pool.
// Call it after http.Server.Shutdown, which does not track hijacked connections.
func (a *App) Shutdown(ctx context.Context) error {
	err := a.hub.Shutdown(ctx)
	a.pool.Close()
	return err
}

// originHosts converts "https://app.example.com" into the host patterns the
// WebSocket library matches the Origin header against.
func originHosts(origins []string) []string {
	var hosts []string
	for _, o := range origins {
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			hosts = append(hosts, u.Host)
		}
	}
	return hosts
}
