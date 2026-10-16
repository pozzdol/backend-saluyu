package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
)

var (
	ErrRefreshNotFound = errors.New("refresh token not found")
	ErrRefreshReused   = errors.New("refresh token already used")
	ErrSessionNotFound = errors.New("session not found")
)

// SessionRepository stores refresh tokens by hash. Rotation keeps session_id, so all tokens
// of one login form one session (one device in the list).
// ponytail: expired rows are never deleted; add a periodic DELETE when the table gets big.
type SessionRepository struct {
	db *sql.DB
}

func NewSessionRepository(db *sql.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(ctx context.Context, userID, sessionID uuid.UUID, hash []byte, expiresAt time.Time, userAgent string) error {
	_, err := r.db.ExecContext(ctx, `
	INSERT INTO refresh_tokens (user_id, session_id, token_hash, expires_at, user_agent) VALUES ($1, $2, $3, $4, $5)`,
		userID, sessionID, hash, expiresAt, userAgent)
	return err
}

// Consume revokes a live token and returns its owner and session in one statement, so two
// concurrent refreshes with the same token cannot both succeed. A token that was rotated
// before returns its owner with ErrRefreshReused, so the caller can end that user's sessions.
// A token whose session was ended on purpose (logout, revoke) is just ErrRefreshNotFound.
func (r *SessionRepository) Consume(ctx context.Context, hash []byte) (userID, sessionID uuid.UUID, err error) {
	err = r.db.QueryRowContext(ctx, `
	UPDATE refresh_tokens SET revoked_at = now()
	WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()
	RETURNING user_id, session_id`, hash).Scan(&userID, &sessionID)
	if !errors.Is(err, sql.ErrNoRows) {
		return userID, sessionID, err
	}

	err = r.db.QueryRowContext(ctx,
		`SELECT user_id FROM refresh_tokens WHERE token_hash = $1 AND revoked_at IS NOT NULL AND NOT ended`, hash).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, uuid.Nil, ErrRefreshNotFound // unknown, expired, or ended
	}
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return userID, uuid.Nil, ErrRefreshReused
}

// End closes one session of the user.
func (r *SessionRepository) End(ctx context.Context, userID, sessionID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `
	UPDATE refresh_tokens SET revoked_at = COALESCE(revoked_at, now()), ended = true
	WHERE user_id = $1 AND session_id = $2 AND NOT ended`, userID, sessionID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// RevokeAll ends every session of the user except keep (uuid.Nil keeps none).
func (r *SessionRepository) RevokeAll(ctx context.Context, userID, keep uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
	UPDATE refresh_tokens SET revoked_at = COALESCE(revoked_at, now()), ended = true
	WHERE user_id = $1 AND session_id <> $2 AND NOT ended`, userID, keep)
	return err
}

// SessionOf returns the session a live refresh token belongs to.
func (r *SessionRepository) SessionOf(ctx context.Context, hash []byte) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRowContext(ctx,
		`SELECT session_id FROM refresh_tokens WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()`, hash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, ErrSessionNotFound
	}
	return id, err
}

// List returns the user's live sessions, most recently active first. Every refresh adds a row,
// so the newest row's time is when the device was last active.
func (r *SessionRepository) List(ctx context.Context, userID uuid.UUID) ([]model.SessionInfo, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT session_id, (array_agg(user_agent ORDER BY created_at DESC))[1], min(created_at), max(created_at)
	FROM refresh_tokens
	WHERE user_id = $1
	GROUP BY session_id
	HAVING bool_or(revoked_at IS NULL AND expires_at > now())
	ORDER BY max(created_at) DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.SessionInfo{}
	for rows.Next() {
		var s model.SessionInfo
		if err := rows.Scan(&s.ID, &s.UserAgent, &s.StartedAt, &s.LastActive); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SecurityRepository stores one-time email links and 2FA backup codes.
type SecurityRepository struct {
	db *sql.DB
}

func NewSecurityRepository(db *sql.DB) *SecurityRepository {
	return &SecurityRepository{db: db}
}

var (
	ErrInvalidLink     = errors.New("invalid or expired link")
	ErrEmailUnverified = errors.New("verify your email first")
)

// CreateEmailToken stores a link token; earlier unused links of the same purpose stop working.
func (r *SecurityRepository) CreateEmailToken(ctx context.Context, userID uuid.UUID, purpose string, hash []byte, expiresAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`UPDATE email_tokens SET used_at = now() WHERE user_id = $1 AND purpose = $2 AND used_at IS NULL`, userID, purpose); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO email_tokens (user_id, purpose, token_hash, expires_at) VALUES ($1, $2, $3, $4)`, userID, purpose, hash, expiresAt); err != nil {
		return err
	}
	return tx.Commit()
}

// UseEmailToken marks a live link as used and returns its user, in one statement.
func (r *SecurityRepository) UseEmailToken(ctx context.Context, purpose string, hash []byte) (uuid.UUID, error) {
	var userID uuid.UUID
	err := r.db.QueryRowContext(ctx, `
	UPDATE email_tokens SET used_at = now()
	WHERE token_hash = $1 AND purpose = $2 AND used_at IS NULL AND expires_at > now()
	RETURNING user_id`, hash, purpose).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, ErrInvalidLink
	}
	return userID, err
}

// ReplaceBackupCodes swaps the user's backup codes for new ones (nil hashes = delete all).
func (r *SecurityRepository) ReplaceBackupCodes(ctx context.Context, userID uuid.UUID, hashes [][]byte) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM totp_backup_codes WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for _, h := range hashes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO totp_backup_codes (user_id, code_hash) VALUES ($1, $2)`, userID, h); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UseBackupCode spends one unused code; false when it does not match any.
func (r *SecurityRepository) UseBackupCode(ctx context.Context, userID uuid.UUID, hash []byte) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
	UPDATE totp_backup_codes SET used_at = now()
	WHERE id = (SELECT id FROM totp_backup_codes WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL LIMIT 1)`, userID, hash)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
