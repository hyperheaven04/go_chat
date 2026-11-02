package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/hyperheaven04/go_chat/backend/internal/domain"
)

const (
	DefaultPageSize = 50
	MaxPageSize     = 100
	searchLimit     = 20
)

type ChatService struct {
	users    UserRepository
	dialogs  DialogRepository
	messages MessageRepository
	notifier Notifier
	presence PresenceTracker
	log      *slog.Logger
	now      func() time.Time
}

func NewChatService(
	users UserRepository, dialogs DialogRepository, messages MessageRepository,
	notifier Notifier, presence PresenceTracker, log *slog.Logger,
) *ChatService {
	return &ChatService{
		users: users, dialogs: dialogs, messages: messages,
		notifier: notifier, presence: presence, log: log, now: time.Now,
	}
}

func (s *ChatService) Me(ctx context.Context, userID uuid.UUID) (domain.User, error) {
	return s.users.GetByID(ctx, userID)
}

func (s *ChatService) IsOnline(userID uuid.UUID) bool { return s.presence.IsOnline(userID) }

func (s *ChatService) SearchUsers(ctx context.Context, userID uuid.UUID, query string) ([]domain.User, error) {
	q := domain.NormalizeUsername(query)
	if q == "" {
		return []domain.User{}, nil
	}
	return s.users.Search(ctx, q, userID, searchLimit)
}

// OpenDialog returns the dialog with peerID, creating it on first contact.
func (s *ChatService) OpenDialog(ctx context.Context, userID, peerID uuid.UUID) (domain.DialogSummary, bool, error) {
	if userID == peerID {
		return domain.DialogSummary{}, false, domain.NewValidationError("peer_id", "cannot open a dialog with yourself")
	}
	if _, err := s.users.GetByID(ctx, peerID); err != nil {
		return domain.DialogSummary{}, false, err
	}
	d, created, err := s.dialogs.GetOrCreate(ctx, userID, peerID)
	if err != nil {
		return domain.DialogSummary{}, false, err
	}
	sum, err := s.GetDialog(ctx, userID, d.ID)
	return sum, created, err
}

func (s *ChatService) ListDialogs(ctx context.Context, userID uuid.UUID) ([]domain.DialogSummary, error) {
	list, err := s.dialogs.ListSummaries(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].PeerOnline = s.presence.IsOnline(list[i].Peer.ID)
	}
	return list, nil
}

func (s *ChatService) GetDialog(ctx context.Context, userID, dialogID uuid.UUID) (domain.DialogSummary, error) {
	sum, err := s.dialogs.GetSummary(ctx, userID, dialogID)
	if err != nil {
		return domain.DialogSummary{}, err
	}
	sum.PeerOnline = s.presence.IsOnline(sum.Peer.ID)
	return sum, nil
}

// ListMessages returns a page of history, newest first. beforeID = 0 means the latest page.
func (s *ChatService) ListMessages(ctx context.Context, userID, dialogID uuid.UUID, beforeID int64, limit int) ([]domain.Message, error) {
	if _, err := s.dialogFor(ctx, userID, dialogID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}
	return s.messages.List(ctx, dialogID, beforeID, limit)
}

type SendMessageInput struct {
	DialogID uuid.UUID
	ClientID uuid.UUID
	Body     string
}

func (s *ChatService) SendMessage(ctx context.Context, userID uuid.UUID, in SendMessageInput) (domain.Message, error) {
	if in.ClientID == uuid.Nil {
		return domain.Message{}, domain.NewValidationError("client_id", "is required")
	}
	body, err := domain.NormalizeMessageBody(in.Body)
	if err != nil {
		return domain.Message{}, err
	}
	d, err := s.dialogFor(ctx, userID, in.DialogID)
	if err != nil {
		return domain.Message{}, err
	}
	m, created, err := s.messages.Create(ctx, domain.NewMessage{
		DialogID: d.ID, SenderID: userID, ClientID: in.ClientID, Body: body,
	})
	if err != nil {
		return domain.Message{}, err
	}
	if m.DialogID != d.ID {
		return domain.Message{}, domain.NewValidationError("client_id", "already used for another message")
	}

	recipients := d.Members() // the sender too: their other devices must see it
	if !created {
		// A retry: only the sender needs the confirmation, the peer already has it.
		recipients = []uuid.UUID{userID}
	}
	s.notifier.Publish(recipients, domain.MessageCreatedEvent{Message: m})
	return m, nil
}

func (s *ChatService) MarkRead(ctx context.Context, userID, dialogID uuid.UUID, messageID int64) error {
	if messageID <= 0 {
		return domain.NewValidationError("message_id", "must be positive")
	}
	d, err := s.dialogFor(ctx, userID, dialogID)
	if err != nil {
		return err
	}
	advanced, err := s.messages.MarkRead(ctx, d.ID, userID, messageID)
	if err != nil || !advanced {
		return err
	}
	s.notifier.Publish(d.Members(), domain.MessagesReadEvent{
		DialogID: d.ID, ReaderID: userID, LastReadMsgID: messageID,
	})
	return nil
}

func (s *ChatService) SetTyping(ctx context.Context, userID, dialogID uuid.UUID, active bool) error {
	d, err := s.dialogFor(ctx, userID, dialogID)
	if err != nil {
		return err
	}
	s.notifier.Publish([]uuid.UUID{d.Peer(userID)}, domain.TypingEvent{
		DialogID: d.ID, UserID: userID, Active: active,
	})
	return nil
}

// UserConnected is called when a user's first live connection opens.
func (s *ChatService) UserConnected(ctx context.Context, userID uuid.UUID) {
	s.broadcastPresence(ctx, domain.PresenceEvent{UserID: userID, Online: true})
}

// UserDisconnected is called when a user's last live connection closes.
func (s *ChatService) UserDisconnected(ctx context.Context, userID uuid.UUID) {
	now := s.now().UTC()
	if err := s.users.TouchLastSeen(ctx, userID, now); err != nil {
		s.log.ErrorContext(ctx, "update last seen", "user_id", userID, "error", err)
	}
	// The user may have reconnected while we were writing to the database.
	if s.presence.IsOnline(userID) {
		return
	}
	s.broadcastPresence(ctx, domain.PresenceEvent{UserID: userID, Online: false, LastSeenAt: &now})
}

func (s *ChatService) broadcastPresence(ctx context.Context, ev domain.PresenceEvent) {
	peers, err := s.dialogs.PeerIDs(ctx, ev.UserID)
	if err != nil {
		s.log.ErrorContext(ctx, "load presence subscribers", "user_id", ev.UserID, "error", err)
		return
	}
	if len(peers) > 0 {
		s.notifier.Publish(peers, ev)
	}
}

// dialogFor loads a dialog and checks that the user participates in it.
// Non-members get ErrNotFound rather than "forbidden", so the existence of
// other people's dialogs is not disclosed.
func (s *ChatService) dialogFor(ctx context.Context, userID, dialogID uuid.UUID) (domain.Dialog, error) {
	d, err := s.dialogs.Get(ctx, dialogID)
	if err != nil {
		return domain.Dialog{}, err
	}
	if !d.HasMember(userID) {
		return domain.Dialog{}, domain.ErrNotFound
	}
	return d, nil
}
