package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/hyperheaven04/go_chat/backend/internal/domain"
	"github.com/hyperheaven04/go_chat/backend/internal/service"
	"github.com/hyperheaven04/go_chat/backend/internal/transport/dto"
)

func (a *API) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.DB.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type credentialsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authResponse struct {
	User      dto.User  `json:"user"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func toAuthResponse(res service.AuthResult) authResponse {
	return authResponse{User: dto.FromUser(res.User), Token: res.Token, ExpiresAt: res.ExpiresAt.UTC()}
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := decodeJSON(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}
	res, err := a.Auth.Register(r.Context(), req.Username, req.Password)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAuthResponse(res))
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := decodeJSON(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}
	res, err := a.Auth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAuthResponse(res))
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	u, err := a.Chat.Me(r.Context(), userIDFrom(r.Context()))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.FromUser(u))
}

func (a *API) searchUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.Chat.SearchUsers(r.Context(), userIDFrom(r.Context()), r.URL.Query().Get("q"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	out := make([]dto.User, len(users))
	for i, u := range users {
		out[i] = dto.FromUser(u)
		online := a.Chat.IsOnline(u.ID)
		out[i].Online = &online
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

func (a *API) listDialogs(w http.ResponseWriter, r *http.Request) {
	list, err := a.Chat.ListDialogs(r.Context(), userIDFrom(r.Context()))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	out := make([]dto.Dialog, len(list))
	for i, d := range list {
		out[i] = dto.FromDialog(d)
	}
	writeJSON(w, http.StatusOK, map[string]any{"dialogs": out})
}

type openDialogRequest struct {
	PeerID uuid.UUID `json:"peer_id"`
}

func (a *API) openDialog(w http.ResponseWriter, r *http.Request) {
	var req openDialogRequest
	if err := decodeJSON(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}
	d, created, err := a.Chat.OpenDialog(r.Context(), userIDFrom(r.Context()), req.PeerID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, dto.FromDialog(d))
}

func (a *API) getDialog(w http.ResponseWriter, r *http.Request) {
	dialogID, err := pathUUID(r, "id")
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	d, err := a.Chat.GetDialog(r.Context(), userIDFrom(r.Context()), dialogID)
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.FromDialog(d))
}

// listMessages: GET /dialogs/{id}/messages?before_id=123&limit=50 — newest first.
func (a *API) listMessages(w http.ResponseWriter, r *http.Request) {
	dialogID, err := pathUUID(r, "id")
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	q := r.URL.Query()
	beforeID, err := queryInt(q.Get("before_id"), "before_id")
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	limit, err := queryInt(q.Get("limit"), "limit")
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	msgs, err := a.Chat.ListMessages(r.Context(), userIDFrom(r.Context()), dialogID, beforeID, int(limit))
	if err != nil {
		a.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": dto.FromMessages(msgs)})
}

func (a *API) issueTicket(w http.ResponseWriter, r *http.Request) {
	t, ttl := a.Tickets.Issue(userIDFrom(r.Context()))
	writeJSON(w, http.StatusOK, map[string]any{"ticket": t, "expires_in": int(ttl.Seconds())})
}

func pathUUID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		// A malformed ID can't exist, so it's indistinguishable from a missing one.
		return uuid.Nil, domain.ErrNotFound
	}
	return id, nil
}

func queryInt(s, field string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, domain.NewValidationError(field, "must be a non-negative integer")
	}
	return n, nil
}
