package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/hyperheaven04/go_chat/backend/internal/domain"
)

// --- fakes -------------------------------------------------------------------

type fakeUsers struct {
	byID   map[uuid.UUID]domain.User
	hashes map[string]string
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{byID: map[uuid.UUID]domain.User{}, hashes: map[string]string{}}
}

func (f *fakeUsers) add(name string) domain.User {
	u := domain.User{ID: uuid.New(), Username: name}
	f.byID[u.ID] = u
	return u
}

func (f *fakeUsers) Create(_ context.Context, username, hash string) (domain.User, error) {
	if _, ok := f.hashes[username]; ok {
		return domain.User{}, domain.ErrUsernameTaken
	}
	u := f.add(username)
	f.hashes[username] = hash
	return u, nil
}

func (f *fakeUsers) GetByID(_ context.Context, id uuid.UUID) (domain.User, error) {
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return domain.User{}, domain.ErrNotFound
}

func (f *fakeUsers) GetCredentials(_ context.Context, username string) (domain.User, string, error) {
	for _, u := range f.byID {
		if u.Username == username {
			return u, f.hashes[username], nil
		}
	}
	return domain.User{}, "", domain.ErrNotFound
}

func (f *fakeUsers) Search(context.Context, string, uuid.UUID, int) ([]domain.User, error) {
	return nil, nil
}
func (f *fakeUsers) TouchLastSeen(context.Context, uuid.UUID, time.Time) error { return nil }

type fakeDialogs struct{ byID map[uuid.UUID]domain.Dialog }

func (f *fakeDialogs) GetOrCreate(_ context.Context, a, b uuid.UUID) (domain.Dialog, bool, error) {
	for _, d := range f.byID {
		if d.HasMember(a) && d.HasMember(b) {
			return d, false, nil
		}
	}
	d := domain.Dialog{ID: uuid.New(), UserA: a, UserB: b}
	f.byID[d.ID] = d
	return d, true, nil
}

func (f *fakeDialogs) Get(_ context.Context, id uuid.UUID) (domain.Dialog, error) {
	if d, ok := f.byID[id]; ok {
		return d, nil
	}
	return domain.Dialog{}, domain.ErrNotFound
}

func (f *fakeDialogs) PeerIDs(_ context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	var out []uuid.UUID
	for _, d := range f.byID {
		if d.HasMember(id) {
			out = append(out, d.Peer(id))
		}
	}
	return out, nil
}

func (f *fakeDialogs) ListSummaries(context.Context, uuid.UUID) ([]domain.DialogSummary, error) {
	return nil, nil
}

func (f *fakeDialogs) GetSummary(_ context.Context, _, id uuid.UUID) (domain.DialogSummary, error) {
	d, err := f.Get(context.Background(), id)
	return domain.DialogSummary{Dialog: d}, err
}

type fakeMessages struct {
	msgs     []domain.Message
	lastRead map[uuid.UUID]int64
}

func (f *fakeMessages) Create(_ context.Context, in domain.NewMessage) (domain.Message, bool, error) {
	for _, m := range f.msgs {
		if m.SenderID == in.SenderID && m.ClientID == in.ClientID {
			return m, false, nil
		}
	}
	m := domain.Message{
		ID: int64(len(f.msgs) + 1), DialogID: in.DialogID, SenderID: in.SenderID,
		ClientID: in.ClientID, Body: in.Body, CreatedAt: time.Now(),
	}
	f.msgs = append(f.msgs, m)
	return m, true, nil
}

func (f *fakeMessages) List(context.Context, uuid.UUID, int64, int) ([]domain.Message, error) {
	return f.msgs, nil
}

func (f *fakeMessages) MarkRead(_ context.Context, _, userID uuid.UUID, id int64) (bool, error) {
	if f.lastRead[userID] >= id {
		return false, nil
	}
	f.lastRead[userID] = id
	return true, nil
}

type published struct {
	to []uuid.UUID
	ev domain.Event
}

type fakeNotifier struct {
	mu     sync.Mutex
	events []published
	online map[uuid.UUID]bool
}

func (f *fakeNotifier) Publish(to []uuid.UUID, ev domain.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, published{to, ev})
}

func (f *fakeNotifier) IsOnline(id uuid.UUID) bool { return f.online[id] }

type fakeHasher struct{ dummyCalls int }

func (h *fakeHasher) Hash(p string) (string, error) { return "hashed:" + p, nil }
func (h *fakeHasher) Compare(hash, p string) bool   { return hash == "hashed:"+p }
func (h *fakeHasher) CompareDummy(string)           { h.dummyCalls++ }

type fakeTokens struct{}

func (fakeTokens) Issue(id uuid.UUID) (string, time.Time, error) {
	return "token-" + id.String(), time.Now().Add(time.Hour), nil
}

// --- fixture -----------------------------------------------------------------

type fixture struct {
	chat            *ChatService
	users           *fakeUsers
	messages        *fakeMessages
	notifier        *fakeNotifier
	alice, bob, eve domain.User
	dialog          domain.Dialog
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	users := newFakeUsers()
	dialogs := &fakeDialogs{byID: map[uuid.UUID]domain.Dialog{}}
	messages := &fakeMessages{lastRead: map[uuid.UUID]int64{}}
	n := &fakeNotifier{online: map[uuid.UUID]bool{}}
	f := &fixture{
		chat:     NewChatService(users, dialogs, messages, n, n, slog.New(slog.NewTextHandler(io.Discard, nil))),
		users:    users,
		messages: messages,
		notifier: n,
		alice:    users.add("alice"),
		bob:      users.add("bob"),
		eve:      users.add("eve"),
	}
	f.dialog, _, _ = dialogs.GetOrCreate(context.Background(), f.alice.ID, f.bob.ID)
	return f
}

func (f *fixture) send(from domain.User, clientID uuid.UUID, body string) (domain.Message, error) {
	return f.chat.SendMessage(context.Background(), from.ID, SendMessageInput{
		DialogID: f.dialog.ID, ClientID: clientID, Body: body,
	})
}

// --- tests -------------------------------------------------------------------

func TestSendMessageNotifiesBothParticipants(t *testing.T) {
	f := newFixture(t)
	m, err := f.send(f.alice, uuid.New(), "  hello  ")
	if err != nil {
		t.Fatal(err)
	}
	if m.Body != "hello" {
		t.Errorf("body not trimmed: %q", m.Body)
	}
	if len(f.notifier.events) != 1 || len(f.notifier.events[0].to) != 2 {
		t.Fatalf("want one event to both members, got %+v", f.notifier.events)
	}
}

func TestSendMessageIsIdempotent(t *testing.T) {
	f := newFixture(t)
	cid := uuid.New()
	first, _ := f.send(f.alice, cid, "hello")
	second, err := f.send(f.alice, cid, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || len(f.messages.msgs) != 1 {
		t.Fatal("retry created a duplicate message")
	}
	retry := f.notifier.events[1]
	if len(retry.to) != 1 || retry.to[0] != f.alice.ID {
		t.Fatalf("retry must notify only the sender, got %v", retry.to)
	}
}

func TestOutsiderCannotAccessDialog(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	checks := map[string]error{
		"send":      func() error { _, err := f.send(f.eve, uuid.New(), "hi"); return err }(),
		"history":   func() error { _, err := f.chat.ListMessages(ctx, f.eve.ID, f.dialog.ID, 0, 10); return err }(),
		"typing":    f.chat.SetTyping(ctx, f.eve.ID, f.dialog.ID, true),
		"mark read": f.chat.MarkRead(ctx, f.eve.ID, f.dialog.ID, 1),
	}
	for name, err := range checks {
		// Not found rather than forbidden: outsiders learn nothing about the dialog.
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s: err = %v, want ErrNotFound", name, err)
		}
	}
	if len(f.notifier.events) != 0 {
		t.Fatalf("outsider triggered events: %+v", f.notifier.events)
	}
}

func TestTypingGoesOnlyToPeer(t *testing.T) {
	f := newFixture(t)
	if err := f.chat.SetTyping(context.Background(), f.alice.ID, f.dialog.ID, true); err != nil {
		t.Fatal(err)
	}
	ev := f.notifier.events[0]
	if len(ev.to) != 1 || ev.to[0] != f.bob.ID {
		t.Fatalf("typing sent to %v, want only bob", ev.to)
	}
}

func TestMarkReadNotifiesOnlyWhenAdvanced(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	m1, _ := f.send(f.alice, uuid.New(), "one")
	m2, _ := f.send(f.alice, uuid.New(), "two")
	base := len(f.notifier.events)

	_ = f.chat.MarkRead(ctx, f.bob.ID, f.dialog.ID, m2.ID)
	_ = f.chat.MarkRead(ctx, f.bob.ID, f.dialog.ID, m1.ID) // stale, must be ignored
	_ = f.chat.MarkRead(ctx, f.bob.ID, f.dialog.ID, m2.ID) // repeat, must be ignored

	if got := len(f.notifier.events) - base; got != 1 {
		t.Fatalf("want 1 read event, got %d", got)
	}
}

func TestSendMessageValidation(t *testing.T) {
	f := newFixture(t)
	var ve *domain.ValidationError
	if _, err := f.send(f.alice, uuid.Nil, "hi"); !errors.As(err, &ve) {
		t.Errorf("missing client_id: %v", err)
	}
	if _, err := f.send(f.alice, uuid.New(), "   "); !errors.As(err, &ve) {
		t.Errorf("blank body: %v", err)
	}
}

func TestOpenDialogWithSelf(t *testing.T) {
	f := newFixture(t)
	var ve *domain.ValidationError
	if _, _, err := f.chat.OpenDialog(context.Background(), f.alice.ID, f.alice.ID); !errors.As(err, &ve) {
		t.Fatalf("err = %v, want validation error", err)
	}
}

func TestPresenceSkipsOfflineWhenReconnected(t *testing.T) {
	f := newFixture(t)
	f.notifier.online[f.alice.ID] = true // reconnected before disconnect handling finished
	f.chat.UserDisconnected(context.Background(), f.alice.ID)
	if len(f.notifier.events) != 0 {
		t.Fatalf("offline broadcast for an online user: %+v", f.notifier.events)
	}
}

func TestAuth(t *testing.T) {
	users := newFakeUsers()
	hasher := &fakeHasher{}
	s := NewAuthService(users, hasher, fakeTokens{})
	ctx := context.Background()

	if _, err := s.Register(ctx, "Alice", "password1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Register(ctx, "alice", "password2"); !errors.Is(err, domain.ErrUsernameTaken) {
		t.Errorf("case-insensitive duplicate: err = %v", err)
	}
	if res, err := s.Login(ctx, "ALICE", "password1"); err != nil || res.Token == "" {
		t.Errorf("login: %v", err)
	}
	if _, err := s.Login(ctx, "alice", "wrong-pass"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("wrong password: err = %v", err)
	}
	if _, err := s.Login(ctx, "nobody", "password1"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("unknown user: err = %v", err)
	}
	if hasher.dummyCalls != 1 {
		t.Error("unknown user must still spend time on a hash comparison")
	}
}
