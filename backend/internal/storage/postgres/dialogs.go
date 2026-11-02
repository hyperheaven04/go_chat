package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hyperheaven04/go_chat/backend/internal/domain"
)

type DialogRepo struct{ db *pgxpool.Pool }

func NewDialogRepo(db *pgxpool.Pool) *DialogRepo { return &DialogRepo{db: db} }

// GetOrCreate returns the dialog between two users, creating it if needed.
// Safe under concurrency: two users opening the dialog at the same moment
// end up with the same row thanks to the unique (user_a, user_b) constraint.
func (r *DialogRepo) GetOrCreate(ctx context.Context, u1, u2 uuid.UUID) (domain.Dialog, bool, error) {
	a, b := u1, u2
	if a.String() > b.String() {
		a, b = b, a
	}
	var d domain.Dialog
	err := r.db.QueryRow(ctx,
		`INSERT INTO dialogs (user_a, user_b) VALUES ($1, $2)
		 ON CONFLICT (user_a, user_b) DO NOTHING
		 RETURNING id, user_a, user_b, created_at`, a, b,
	).Scan(&d.ID, &d.UserA, &d.UserB, &d.CreatedAt)
	if err == nil {
		return d, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Dialog{}, false, fmt.Errorf("insert dialog: %w", err)
	}
	err = r.db.QueryRow(ctx,
		`SELECT id, user_a, user_b, created_at FROM dialogs WHERE user_a = $1 AND user_b = $2`, a, b,
	).Scan(&d.ID, &d.UserA, &d.UserB, &d.CreatedAt)
	return d, false, wrapNotFound(err, "select dialog")
}

func (r *DialogRepo) Get(ctx context.Context, id uuid.UUID) (domain.Dialog, error) {
	var d domain.Dialog
	err := r.db.QueryRow(ctx,
		`SELECT id, user_a, user_b, created_at FROM dialogs WHERE id = $1`, id,
	).Scan(&d.ID, &d.UserA, &d.UserB, &d.CreatedAt)
	return d, wrapNotFound(err, "get dialog")
}

// PeerIDs returns everyone the user has a dialog with (presence subscribers).
func (r *DialogRepo) PeerIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.db.Query(ctx,
		`SELECT CASE WHEN user_a = $1 THEN user_b ELSE user_a END
		 FROM dialogs WHERE user_a = $1 OR user_b = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("select peers: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

const summarySQL = `
SELECT d.id, d.user_a, d.user_b, d.created_at,
       p.id, p.username, p.created_at, p.last_seen_at,
       lm.id, lm.sender_id, lm.client_id, lm.body, lm.created_at,
       COALESCE(mr.last_read_message_id, 0),
       COALESCE(pr.last_read_message_id, 0),
       (SELECT count(*) FROM messages m
         WHERE m.dialog_id = d.id AND m.sender_id <> $1
           AND m.id > COALESCE(mr.last_read_message_id, 0))
FROM dialogs d
JOIN users p ON p.id = CASE WHEN d.user_a = $1 THEN d.user_b ELSE d.user_a END
LEFT JOIN LATERAL (
    SELECT id, sender_id, client_id, body, created_at FROM messages
    WHERE dialog_id = d.id ORDER BY id DESC LIMIT 1
) lm ON true
LEFT JOIN dialog_reads mr ON mr.dialog_id = d.id AND mr.user_id = $1
LEFT JOIN dialog_reads pr ON pr.dialog_id = d.id AND pr.user_id = p.id
WHERE (d.user_a = $1 OR d.user_b = $1)`

// ListSummaries returns the user's dialogs, most recently active first.
func (r *DialogRepo) ListSummaries(ctx context.Context, userID uuid.UUID) ([]domain.DialogSummary, error) {
	rows, err := r.db.Query(ctx,
		summarySQL+` ORDER BY COALESCE(lm.created_at, d.created_at) DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list dialogs: %w", err)
	}
	return pgx.CollectRows(rows, scanSummary)
}

// GetSummary returns ErrNotFound both when the dialog does not exist and when
// the user is not a participant, so outsiders cannot probe for dialog IDs.
func (r *DialogRepo) GetSummary(ctx context.Context, userID, dialogID uuid.UUID) (domain.DialogSummary, error) {
	rows, err := r.db.Query(ctx, summarySQL+` AND d.id = $2`, userID, dialogID)
	if err != nil {
		return domain.DialogSummary{}, fmt.Errorf("get dialog summary: %w", err)
	}
	s, err := pgx.CollectExactlyOneRow(rows, scanSummary)
	return s, wrapNotFound(err, "get dialog summary")
}

func scanSummary(row pgx.CollectableRow) (domain.DialogSummary, error) {
	var (
		s        domain.DialogSummary
		lmID     *int64
		lmSender *uuid.UUID
		lmClient *uuid.UUID
		lmBody   *string
		lmAt     *time.Time
	)
	err := row.Scan(
		&s.ID, &s.UserA, &s.UserB, &s.CreatedAt,
		&s.Peer.ID, &s.Peer.Username, &s.Peer.CreatedAt, &s.Peer.LastSeenAt,
		&lmID, &lmSender, &lmClient, &lmBody, &lmAt,
		&s.MyLastReadID, &s.PeerLastReadID, &s.UnreadCount,
	)
	if err != nil {
		return s, err
	}
	if lmID != nil {
		s.LastMessage = &domain.Message{
			ID: *lmID, DialogID: s.ID, SenderID: *lmSender, ClientID: *lmClient,
			Body: *lmBody, CreatedAt: *lmAt,
		}
	}
	return s, nil
}
