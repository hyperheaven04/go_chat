package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/hyperheaven04/go_chat/backend/internal/config"
	"github.com/hyperheaven04/go_chat/backend/internal/transport/ws"
)

// End-to-end tests run the real HTTP + WebSocket stack against PostgreSQL.
// They are skipped unless TEST_DATABASE_URL is set (see `make test-integration`).

func startServer(t *testing.T) *httptest.Server {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	cfg := config.Config{
		DatabaseURL: dsn,
		JWTSecret:   []byte("e2e-secret-e2e-secret-e2e-secret"),
		JWTTTL:      time.Hour,
		WSTicketTTL: 30 * time.Second,
		BcryptCost:  10,
	}
	a, err := New(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler)
	t.Cleanup(func() {
		srv.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.Shutdown(ctx)
	})
	return srv
}

type client struct {
	t     *testing.T
	base  string
	token string
	id    uuid.UUID
	name  string
}

func (c *client) do(method, path string, body any, wantStatus int, out any) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		c.t.Fatalf("%s %s: status %d, want %d: %s", method, path, resp.StatusCode, wantStatus, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatalf("decode %s: %v", raw, err)
		}
	}
}

func register(t *testing.T, base, prefix string) *client {
	t.Helper()
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	c := &client{t: t, base: base, name: prefix + "_" + hex.EncodeToString(b)}
	var res struct {
		Token string `json:"token"`
		User  struct {
			ID uuid.UUID `json:"id"`
		} `json:"user"`
	}
	c.do("POST", "/api/v1/auth/register",
		map[string]string{"username": c.name, "password": "s3cret-pass"},
		http.StatusCreated, &res)
	c.token, c.id = res.Token, res.User.ID
	return c
}

func (c *client) connect() *wsConn {
	c.t.Helper()
	var tk struct {
		Ticket string `json:"ticket"`
	}
	c.do("POST", "/api/v1/ws/ticket", nil, http.StatusOK, &tk)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(c.base, "http")+"/api/v1/ws?ticket="+tk.Ticket, nil)
	if err != nil {
		c.t.Fatal(err)
	}
	c.t.Cleanup(func() { _ = conn.CloseNow() })
	return &wsConn{t: c.t, conn: conn}
}

type wsConn struct {
	t    *testing.T
	conn *websocket.Conn
}

func (w *wsConn) send(typ string, payload any) {
	w.t.Helper()
	b, _ := json.Marshal(map[string]any{"type": typ, "payload": payload})
	if err := w.conn.Write(context.Background(), websocket.MessageText, b); err != nil {
		w.t.Fatal(err)
	}
}

// expect reads frames until one of the given type arrives, skipping others
// (e.g. presence updates that may interleave).
func (w *wsConn) expect(typ string) json.RawMessage {
	w.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, b, err := w.conn.Read(ctx)
		if err != nil {
			w.t.Fatalf("waiting for %q: %v", typ, err)
		}
		var env struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		_ = json.Unmarshal(b, &env)
		if env.Type == typ {
			return env.Payload
		}
	}
}

func TestChatFlow(t *testing.T) {
	srv := startServer(t)
	alice := register(t, srv.URL, "alice")
	bob := register(t, srv.URL, "bob")

	var dialog struct {
		ID uuid.UUID `json:"id"`
	}
	alice.do("POST", "/api/v1/dialogs", map[string]any{"peer_id": bob.id}, http.StatusCreated, &dialog)
	// Opening from the other side returns the same dialog.
	var again struct {
		ID uuid.UUID `json:"id"`
	}
	bob.do("POST", "/api/v1/dialogs", map[string]any{"peer_id": alice.id}, http.StatusOK, &again)
	if again.ID != dialog.ID {
		t.Fatal("same pair got two dialogs")
	}

	aliceWS := alice.connect()
	bobWS := bob.connect()

	// Presence: alice sees bob come online.
	var presence ws.PresencePayload
	_ = json.Unmarshal(aliceWS.expect(ws.TypePresence), &presence)
	if presence.UserID != bob.id || !presence.Online {
		t.Fatalf("presence = %+v", presence)
	}

	// Typing reaches the peer.
	bobWS.send(ws.TypeTyping, map[string]any{"dialog_id": dialog.ID, "active": true})
	var typing ws.TypingPayload
	_ = json.Unmarshal(aliceWS.expect(ws.TypeTyping), &typing)
	if typing.UserID != bob.id || !typing.Active {
		t.Fatalf("typing = %+v", typing)
	}

	// Message: both sides receive it with a server-assigned timestamp.
	before := time.Now().Add(-time.Second)
	clientID := uuid.New()
	aliceWS.send(ws.TypeMessageSend, map[string]any{"dialog_id": dialog.ID, "client_id": clientID, "body": "Привет!"})
	var msg struct {
		ID        int64     `json:"id"`
		ClientID  uuid.UUID `json:"client_id"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
	}
	_ = json.Unmarshal(bobWS.expect(ws.TypeMessageNew), &msg)
	if msg.Body != "Привет!" || msg.CreatedAt.Before(before) || msg.CreatedAt.Location() != time.UTC {
		t.Fatalf("message = %+v", msg)
	}
	aliceWS.expect(ws.TypeMessageNew) // sender's own echo, used to confirm delivery

	// Read receipt reaches the sender.
	bobWS.send(ws.TypeMessageRead, map[string]any{"dialog_id": dialog.ID, "message_id": msg.ID})
	var read ws.ReadPayload
	_ = json.Unmarshal(aliceWS.expect(ws.TypeMessageRead), &read)
	if read.UserID != bob.id || read.MessageID != msg.ID {
		t.Fatalf("read = %+v", read)
	}

	// History via REST.
	var history struct {
		Messages []struct {
			ID int64 `json:"id"`
		} `json:"messages"`
	}
	bob.do("GET", "/api/v1/dialogs/"+dialog.ID.String()+"/messages", nil, http.StatusOK, &history)
	if len(history.Messages) != 1 || history.Messages[0].ID != msg.ID {
		t.Fatalf("history = %+v", history)
	}

	// Errors come back as error frames, the connection stays open.
	aliceWS.send(ws.TypeMessageSend, map[string]any{"dialog_id": dialog.ID, "client_id": uuid.New(), "body": "  "})
	aliceWS.expect(ws.TypeError)

	// Bob goes offline: alice gets presence with last_seen_at.
	_ = bobWS.conn.Close(websocket.StatusNormalClosure, "")
	_ = json.Unmarshal(aliceWS.expect(ws.TypePresence), &presence)
	if presence.Online || presence.LastSeenAt == nil {
		t.Fatalf("offline presence = %+v", presence)
	}
}

func TestSecurity(t *testing.T) {
	srv := startServer(t)
	alice := register(t, srv.URL, "alice")
	bob := register(t, srv.URL, "bob")
	eve := register(t, srv.URL, "eve")

	var dialog struct {
		ID uuid.UUID `json:"id"`
	}
	alice.do("POST", "/api/v1/dialogs", map[string]any{"peer_id": bob.id}, http.StatusCreated, &dialog)

	t.Run("outsider cannot read history", func(t *testing.T) {
		eve.t = t
		eve.do("GET", "/api/v1/dialogs/"+dialog.ID.String()+"/messages", nil, http.StatusNotFound, nil)
		eve.do("GET", "/api/v1/dialogs/"+dialog.ID.String(), nil, http.StatusNotFound, nil)
	})

	t.Run("no token", func(t *testing.T) {
		anon := &client{t: t, base: srv.URL}
		anon.do("GET", "/api/v1/dialogs", nil, http.StatusUnauthorized, nil)
	})

	t.Run("forged token", func(t *testing.T) {
		forged := &client{t: t, base: srv.URL, token: alice.token[:len(alice.token)-3] + "abc"}
		forged.do("GET", "/api/v1/users/me", nil, http.StatusUnauthorized, nil)
	})

	t.Run("websocket requires a valid one-time ticket", func(t *testing.T) {
		url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/ws?ticket="
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, _, err := websocket.Dial(ctx, url+"bogus", nil); err == nil {
			t.Fatal("bogus ticket accepted")
		}

		var tk struct {
			Ticket string `json:"ticket"`
		}
		alice.t = t
		alice.do("POST", "/api/v1/ws/ticket", nil, http.StatusOK, &tk)
		conn, _, err := websocket.Dial(ctx, url+tk.Ticket, nil)
		if err != nil {
			t.Fatal(err)
		}
		conn.CloseNow()
		if _, _, err := websocket.Dial(ctx, url+tk.Ticket, nil); err == nil {
			t.Fatal("ticket reused")
		}
	})

	t.Run("cross-site websocket is rejected", func(t *testing.T) {
		var tk struct {
			Ticket string `json:"ticket"`
		}
		alice.do("POST", "/api/v1/ws/ticket", nil, http.StatusOK, &tk)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/api/v1/ws?ticket="+tk.Ticket,
			&websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"https://evil.example"}}})
		if err == nil {
			t.Fatal("foreign origin accepted")
		}
	})

	t.Run("login does not reveal whether the user exists", func(t *testing.T) {
		anon := &client{t: t, base: srv.URL}
		var e1, e2 map[string]map[string]string
		anon.do("POST", "/api/v1/auth/login", map[string]string{"username": "no_such_user", "password": "whatever1"}, http.StatusUnauthorized, &e1)
		anon.do("POST", "/api/v1/auth/login", map[string]string{"username": eve.name, "password": "whatever1"}, http.StatusUnauthorized, &e2)
		if e1["error"]["message"] != e2["error"]["message"] {
			t.Fatal("different errors for unknown user and wrong password")
		}
	})
}
