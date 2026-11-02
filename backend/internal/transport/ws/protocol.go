// Package ws implements the real-time WebSocket transport.
//
// Every frame is a JSON envelope {"type": ..., "ref": ..., "payload": {...}}.
// "ref" is an optional client-chosen correlation ID echoed back in errors.
//
//	client → server: message.send, typing, message.read
//	server → client: message.new, typing, message.read, presence, error
package ws

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/hyperheaven04/go_chat/backend/internal/domain"
	"github.com/hyperheaven04/go_chat/backend/internal/transport/dto"
)

const (
	TypeMessageSend = "message.send"
	TypeMessageNew  = "message.new"
	TypeTyping      = "typing"
	TypeMessageRead = "message.read"
	TypePresence    = "presence"
	TypeError       = "error"
)

type Inbound struct {
	Type    string          `json:"type"`
	Ref     string          `json:"ref,omitempty"`
	Payload json.RawMessage `json:"payload"`
}

type Outbound struct {
	Type    string `json:"type"`
	Ref     string `json:"ref,omitempty"`
	Payload any    `json:"payload"`
}

type SendPayload struct {
	DialogID uuid.UUID `json:"dialog_id"`
	ClientID uuid.UUID `json:"client_id"`
	Body     string    `json:"body"`
}

type TypingPayload struct {
	DialogID uuid.UUID `json:"dialog_id"`
	UserID   uuid.UUID `json:"user_id,omitzero"`
	Active   bool      `json:"active"`
}

type ReadPayload struct {
	DialogID  uuid.UUID `json:"dialog_id"`
	UserID    uuid.UUID `json:"user_id,omitzero"`
	MessageID int64     `json:"message_id"`
}

type PresencePayload struct {
	UserID     uuid.UUID  `json:"user_id"`
	Online     bool       `json:"online"`
	LastSeenAt *time.Time `json:"last_seen_at"`
}

func encodeEvent(ev domain.Event) ([]byte, error) {
	var out Outbound
	switch e := ev.(type) {
	case domain.MessageCreatedEvent:
		out = Outbound{Type: TypeMessageNew, Payload: dto.FromMessage(e.Message)}
	case domain.TypingEvent:
		out = Outbound{Type: TypeTyping, Payload: TypingPayload{DialogID: e.DialogID, UserID: e.UserID, Active: e.Active}}
	case domain.MessagesReadEvent:
		out = Outbound{Type: TypeMessageRead, Payload: ReadPayload{DialogID: e.DialogID, UserID: e.ReaderID, MessageID: e.LastReadMsgID}}
	case domain.PresenceEvent:
		out = Outbound{Type: TypePresence, Payload: PresencePayload{UserID: e.UserID, Online: e.Online, LastSeenAt: e.LastSeenAt}}
	default:
		return nil, fmt.Errorf("unknown event %T", ev)
	}
	return json.Marshal(out)
}
