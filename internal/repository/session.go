package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrRefreshNotFound = errors.New("refresh token not found")
	ErrRefreshReused   = errors.New("refresh token already used")
)

// SessionRepository stores refresh tokens by hash.
// ponytail: expired rows are never deleted; add a periodic DELETE when the table gets big.
type SessionRepository struct {
	db *sql.DB
}

func NewSessionRepository(db *sql.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(ctx context.Context, userID uuid.UUID, hash []byte, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`, userID, hash, expiresAt)
	return err
}

// Consume revokes a live token and returns its owner in one statement, so two concurrent
// refreshes with the same token cannot both succeed. A token that was already revoked
// returns its owner with ErrRefreshReused, so the caller can end that user's sessions.
func (r *SessionRepository) Consume(ctx context.Context, hash []byte) (uuid.UUID, error) {
	var userID uuid.UUID
	err := r.db.QueryRowContext(ctx, `
	UPDATE refresh_tokens SET revoked_at = now()
	WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()
	RETURNING user_id`, hash).Scan(&userID)
	if !errors.Is(err, sql.ErrNoRows) {
		return userID, err
	}

	err = r.db.QueryRowContext(ctx,
		`SELECT user_id FROM refresh_tokens WHERE token_hash = $1 AND revoked_at IS NOT NULL`, hash).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, ErrRefreshNotFound // unknown or expired
	}
	if err != nil {
		return uuid.Nil, err
	}
	return userID, ErrRefreshReused
}

func (r *SessionRepository) RevokeAll(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return err
}
