package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hyperheaven04/go_chat/backend/internal/domain"
)

const pgUniqueViolation = "23505"

type UserRepo struct{ db *pgxpool.Pool }

func NewUserRepo(db *pgxpool.Pool) *UserRepo { return &UserRepo{db: db} }

const userColumns = `id, username, created_at, last_seen_at`

func scanUser(row pgx.Row) (domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.Username, &u.CreatedAt, &u.LastSeenAt)
	return u, err
}

func (r *UserRepo) Create(ctx context.Context, username, passwordHash string) (domain.User, error) {
	u, err := scanUser(r.db.QueryRow(ctx,
		`INSERT INTO users (username, password_hash) VALUES ($1, $2) RETURNING `+userColumns,
		username, passwordHash))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return domain.User{}, domain.ErrUsernameTaken
		}
		return domain.User{}, fmt.Errorf("insert user: %w", err)
	}
	return u, nil
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	u, err := scanUser(r.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
	return u, wrapNotFound(err, "get user")
}

func (r *UserRepo) GetCredentials(ctx context.Context, username string) (domain.User, string, error) {
	var (
		u    domain.User
		hash string
	)
	err := r.db.QueryRow(ctx,
		`SELECT `+userColumns+`, password_hash FROM users WHERE username = $1`, username,
	).Scan(&u.ID, &u.Username, &u.CreatedAt, &u.LastSeenAt, &hash)
	return u, hash, wrapNotFound(err, "get credentials")
}

// Search finds users by username prefix, excluding the caller.
func (r *UserRepo) Search(ctx context.Context, prefix string, exclude uuid.UUID, limit int) ([]domain.User, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+userColumns+` FROM users
		 WHERE username LIKE $1 ESCAPE '\' AND id <> $2
		 ORDER BY username LIMIT $3`,
		escapeLike(prefix)+"%", exclude, limit)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.User, error) { return scanUser(row) })
}

func (r *UserRepo) TouchLastSeen(ctx context.Context, id uuid.UUID, at time.Time) error {
	if _, err := r.db.Exec(ctx, `UPDATE users SET last_seen_at = $2 WHERE id = $1`, id, at); err != nil {
		return fmt.Errorf("update last_seen_at: %w", err)
	}
	return nil
}

// escapeLike prevents user input from being interpreted as LIKE wildcards.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func wrapNotFound(err error, op string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return domain.ErrNotFound
	default:
		return fmt.Errorf("%s: %w", op, err)
	}
}
