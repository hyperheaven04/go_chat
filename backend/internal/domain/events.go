package domain

import (
	"time"

	"github.com/google/uuid"
)

// Event is a real-time notification delivered to connected users.
// The set of events is closed: only types in this package implement it.
type Event interface{ isEvent() }

type MessageCreatedEvent struct{ Message Message }

// TypingEvent is ephemeral: it is never persisted. Clients treat it as
// expiring after a few seconds unless refreshed, so a lost "stop" never
// leaves the indicator stuck.
type TypingEvent struct {
	DialogID uuid.UUID
	UserID   uuid.UUID
	Active   bool
}

type MessagesReadEvent struct {
	DialogID      uuid.UUID
	ReaderID      uuid.UUID
	LastReadMsgID int64
}

type PresenceEvent struct {
	UserID     uuid.UUID
	Online     bool
	LastSeenAt *time.Time
}

func (MessageCreatedEvent) isEvent() {}
func (TypingEvent) isEvent()         {}
func (MessagesReadEvent) isEvent()   {}
func (PresenceEvent) isEvent()       {}
