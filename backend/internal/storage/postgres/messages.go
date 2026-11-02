package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hyperheaven04/go_chat/backend/internal/domain"
)

type MessageRepo struct{ db *pgxpool.Pool }

func NewMessageRepo(db *pgxpool.Pool) *MessageRepo { return &MessageRepo{db: db} }

const messageColumns = `id, dialog_id, sender_id, client_id, body, created_at`

func scanMessage(row pgx.Row) (domain.Message, error) {
	var m domain.Message
	err := row.Scan(&m.ID, &m.DialogID, &m.SenderID, &m.ClientID, &m.Body, &m.CreatedAt)
	return m, err
}

// Create inserts a message. If the sender already sent a message with the same
// client_id, the existing message is returned with created=false.
func (r *MessageRepo) Create(ctx context.Context, in domain.NewMessage) (domain.Message, bool, error) {
	m, err := scanMessage(r.db.QueryRow(ctx,
		`INSERT INTO messages (dialog_id, sender_id, client_id, body) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (sender_id, client_id) DO NOTHING
		 RETURNING `+messageColumns,
		in.DialogID, in.SenderID, in.ClientID, in.Body))
	if err == nil {
		return m, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Message{}, false, fmt.Errorf("insert message: %w", err)
	}
	m, err = scanMessage(r.db.QueryRow(ctx,
		`SELECT `+messageColumns+` FROM messages WHERE sender_id = $1 AND client_id = $2`,
		in.SenderID, in.ClientID))
	if err != nil {
		return domain.Message{}, false, fmt.Errorf("select duplicate message: %w", err)
	}
	return m, false, nil
}

// List returns up to limit messages older than beforeID (0 = latest), newest first.
func (r *MessageRepo) List(ctx context.Context, dialogID uuid.UUID, beforeID int64, limit int) ([]domain.Message, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+messageColumns+` FROM messages
		 WHERE dialog_id = $1 AND ($2 = 0 OR id < $2)
		 ORDER BY id DESC LIMIT $3`, dialogID, beforeID, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Message, error) { return scanMessage(row) })
}

// MarkRead moves the user's read pointer forward to messageID. The pointer
// never moves backwards, and messageID must belong to the dialog, so clients
// cannot mark messages that do not exist yet. Returns advanced=false when
// nothing changed.
func (r *MessageRepo) MarkRead(ctx context.Context, dialogID, userID uuid.UUID, messageID int64) (bool, error) {
	tag, err := r.db.Exec(ctx,
		`INSERT INTO dialog_reads (dialog_id, user_id, last_read_message_id)
		 SELECT dialog_id, $2, id FROM messages WHERE dialog_id = $1 AND id = $3
		 ON CONFLICT (dialog_id, user_id) DO UPDATE
		   SET last_read_message_id = EXCLUDED.last_read_message_id, updated_at = now()
		   WHERE dialog_reads.last_read_message_id < EXCLUDED.last_read_message_id`,
		dialogID, userID, messageID)
	if err != nil {
		return false, fmt.Errorf("mark read: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
