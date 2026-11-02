// Package dto defines the JSON representation shared by the REST API and the
// WebSocket protocol, and the mapping of domain errors to client-facing codes.
package dto

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/hyperheaven04/go_chat/backend/internal/domain"
)

type User struct {
	ID         uuid.UUID  `json:"id"`
	Username   string     `json:"username"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	Online     *bool      `json:"online,omitempty"`
}

type Message struct {
	ID        int64     `json:"id"`
	DialogID  uuid.UUID `json:"dialog_id"`
	SenderID  uuid.UUID `json:"sender_id"`
	ClientID  uuid.UUID `json:"client_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type Dialog struct {
	ID             uuid.UUID `json:"id"`
	Peer           User      `json:"peer"`
	LastMessage    *Message  `json:"last_message"`
	UnreadCount    int       `json:"unread_count"`
	MyLastReadID   int64     `json:"my_last_read_id"`
	PeerLastReadID int64     `json:"peer_last_read_id"`
	CreatedAt      time.Time `json:"created_at"`
}

// All timestamps leave the server in UTC; clients convert to local time.

func FromUser(u domain.User) User {
	return User{ID: u.ID, Username: u.Username, CreatedAt: u.CreatedAt.UTC(), LastSeenAt: utcPtr(u.LastSeenAt)}
}

func FromMessage(m domain.Message) Message {
	return Message{
		ID: m.ID, DialogID: m.DialogID, SenderID: m.SenderID, ClientID: m.ClientID,
		Body: m.Body, CreatedAt: m.CreatedAt.UTC(),
	}
}

func FromMessages(ms []domain.Message) []Message {
	out := make([]Message, len(ms))
	for i, m := range ms {
		out[i] = FromMessage(m)
	}
	return out
}

func FromDialog(s domain.DialogSummary) Dialog {
	peer := FromUser(s.Peer)
	peer.Online = &s.PeerOnline
	d := Dialog{
		ID: s.ID, Peer: peer, UnreadCount: s.UnreadCount,
		MyLastReadID: s.MyLastReadID, PeerLastReadID: s.PeerLastReadID, CreatedAt: s.CreatedAt.UTC(),
	}
	if s.LastMessage != nil {
		m := FromMessage(*s.LastMessage)
		d.LastMessage = &m
	}
	return d
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// Error is the client-facing error body. Internal details never leak into it.
type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func ClassifyError(err error) Error {
	var ve *domain.ValidationError
	switch {
	case errors.As(err, &ve):
		return Error{http.StatusBadRequest, "validation_error", ve.Message, ve.Field}
	case errors.Is(err, domain.ErrUsernameTaken):
		return Error{http.StatusConflict, "username_taken", err.Error(), "username"}
	case errors.Is(err, domain.ErrInvalidCredentials):
		return Error{http.StatusUnauthorized, "invalid_credentials", err.Error(), ""}
	case errors.Is(err, domain.ErrUnauthorized):
		return Error{http.StatusUnauthorized, "unauthorized", "authentication required", ""}
	case errors.Is(err, domain.ErrNotFound):
		return Error{http.StatusNotFound, "not_found", "resource not found", ""}
	default:
		return Error{http.StatusInternalServerError, "internal_error", "internal server error", ""}
	}
}
