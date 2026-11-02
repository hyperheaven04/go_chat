package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"github.com/hyperheaven04/go_chat/backend/internal/domain"
	"github.com/hyperheaven04/go_chat/backend/internal/service"
	"github.com/hyperheaven04/go_chat/backend/internal/transport/dto"
)

const (
	sendQueueSize  = 64
	maxFrameSize   = 16 << 10 // a 4000-character message fits comfortably
	writeTimeout   = 10 * time.Second
	pingInterval   = 25 * time.Second
	handlerTimeout = 5 * time.Second

	// Per-connection inbound limit: generous for a human, stops floods.
	inboundRate  = rate.Limit(10)
	inboundBurst = 20
)

var errSlowConsumer = errors.New("slow consumer")

// ChatAPI is the subset of the chat service used by the WebSocket transport.
type ChatAPI interface {
	SendMessage(ctx context.Context, userID uuid.UUID, in service.SendMessageInput) (domain.Message, error)
	MarkRead(ctx context.Context, userID, dialogID uuid.UUID, messageID int64) error
	SetTyping(ctx context.Context, userID, dialogID uuid.UUID, active bool) error
	UserConnected(ctx context.Context, userID uuid.UUID)
	UserDisconnected(ctx context.Context, userID uuid.UUID)
}

// Client is one WebSocket connection. It runs two goroutines: the reader
// handles inbound frames sequentially (so a user's messages keep their
// order), the writer is the only one writing to the socket.
type Client struct {
	userID  uuid.UUID
	conn    *websocket.Conn
	chat    ChatAPI
	send    chan []byte
	limiter *rate.Limiter
	log     *slog.Logger

	parent context.Context // hub context: cancelled on server shutdown
	ctx    context.Context
	cancel context.CancelCauseFunc
}

func newClient(parent context.Context, userID uuid.UUID, conn *websocket.Conn, chat ChatAPI, log *slog.Logger) *Client {
	ctx, cancel := context.WithCancelCause(parent)
	return &Client{
		userID:  userID,
		conn:    conn,
		chat:    chat,
		send:    make(chan []byte, sendQueueSize),
		limiter: rate.NewLimiter(inboundRate, inboundBurst),
		log:     log.With("user_id", userID),
		parent:  parent,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// enqueue never blocks. A client that cannot keep up is disconnected rather
// than allowed to stall everyone else; it will resync via REST on reconnect.
func (c *Client) enqueue(b []byte) {
	select {
	case c.send <- b:
	default:
		c.cancel(errSlowConsumer)
	}
}

// run blocks until the connection is closed.
func (c *Client) run() {
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		c.writeLoop()
	}()
	c.readLoop()
	c.cancel(nil)
	<-writerDone
}

func (c *Client) readLoop() {
	for {
		typ, data, err := c.conn.Read(c.ctx)
		if err != nil {
			if s := websocket.CloseStatus(err); s != websocket.StatusNormalClosure && s != websocket.StatusGoingAway && c.ctx.Err() == nil {
				c.log.Debug("websocket read", "error", err)
			}
			return
		}
		if typ != websocket.MessageText {
			c.replyError("", domain.NewValidationError("frame", "only text frames are supported"))
			continue
		}
		var in Inbound
		if err := json.Unmarshal(data, &in); err != nil {
			c.replyError("", domain.NewValidationError("frame", "malformed JSON"))
			continue
		}
		if !c.limiter.Allow() {
			c.reply(Outbound{Type: TypeError, Ref: in.Ref, Payload: dto.Error{Code: "rate_limited", Message: "too many requests"}})
			continue
		}
		c.handle(in)
	}
}

func (c *Client) handle(in Inbound) {
	ctx, cancel := context.WithTimeout(c.ctx, handlerTimeout)
	defer cancel()

	var err error
	switch in.Type {
	case TypeMessageSend:
		var p SendPayload
		if err = decodePayload(in.Payload, &p); err == nil {
			_, err = c.chat.SendMessage(ctx, c.userID, service.SendMessageInput{DialogID: p.DialogID, ClientID: p.ClientID, Body: p.Body})
		}
	case TypeTyping:
		var p TypingPayload
		if err = decodePayload(in.Payload, &p); err == nil {
			err = c.chat.SetTyping(ctx, c.userID, p.DialogID, p.Active)
		}
	case TypeMessageRead:
		var p ReadPayload
		if err = decodePayload(in.Payload, &p); err == nil {
			err = c.chat.MarkRead(ctx, c.userID, p.DialogID, p.MessageID)
		}
	default:
		err = domain.NewValidationError("type", "unknown message type")
	}
	if err != nil {
		c.replyError(in.Ref, err)
	}
}

func decodePayload(raw json.RawMessage, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return domain.NewValidationError("payload", "invalid payload")
	}
	return nil
}

func (c *Client) replyError(ref string, err error) {
	e := dto.ClassifyError(err)
	if e.Status >= 500 {
		c.log.Error("websocket handler", "error", err)
	}
	c.reply(Outbound{Type: TypeError, Ref: ref, Payload: e})
}

func (c *Client) reply(out Outbound) {
	b, err := json.Marshal(out)
	if err != nil {
		c.log.Error("encode reply", "error", err)
		return
	}
	c.enqueue(b)
}

func (c *Client) writeLoop() {
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	for {
		select {
		case <-c.ctx.Done():
			c.close()
			return
		case b := <-c.send:
			if err := c.write(b); err != nil {
				c.cancel(err)
			}
		case <-ping.C:
			// Ping detects dead peers (e.g. a phone that lost network) that
			// would otherwise look online forever.
			ctx, cancel := context.WithTimeout(c.ctx, writeTimeout)
			err := c.conn.Ping(ctx)
			cancel()
			if err != nil {
				c.cancel(err)
			}
		}
	}
}

func (c *Client) write(b []byte) error {
	ctx, cancel := context.WithTimeout(c.ctx, writeTimeout)
	defer cancel()
	return c.conn.Write(ctx, websocket.MessageText, b)
}

func (c *Client) close() {
	switch {
	case c.parent.Err() != nil:
		_ = c.conn.Close(websocket.StatusGoingAway, "server shutting down")
	case errors.Is(context.Cause(c.ctx), errSlowConsumer):
		_ = c.conn.Close(websocket.StatusPolicyViolation, "slow consumer")
	default:
		_ = c.conn.CloseNow()
	}
}
