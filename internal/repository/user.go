package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pozzdol/backend-saluyu/internal/model"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrEmailTaken   = errors.New("email already taken")
)

const userColumns = `id, name, email, password, img_url, is_active, password_changed_at, start_date, period_start_day, locale, email_verified_at, sessions_revoked_at, totp_secret, totp_enabled_at, created_at, updated_at`

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create inserts the user and seeds their default chart of accounts in one transaction,
// so a failed register never leaves a user without accounts.
func (r *UserRepository) Create(ctx context.Context, user *model.User) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after Commit

	query := `
	INSERT INTO users (name, email, password, img_url, is_active, password_changed_at)
	VALUES ($1, $2, $3, $4, $5, $6)
	RETURNING id, period_start_day, created_at, updated_at
	`
	err = tx.QueryRowContext(ctx, query, user.Name, user.Email, user.Password, user.Image, user.IsActive, user.PasswordChangedAt).
		Scan(&user.ID, &user.PeriodStartDay, &user.CreatedAt, &user.UpdatedAt)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
		return ErrEmailTaken
	}
	if err != nil {
		return err
	}

	if err := seedAccounts(ctx, tx, user.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE lower(email) = lower($1)` // matches users_email_lower_key index
	return scanUser(r.db.QueryRowContext(ctx, query, email))
}

func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE id = $1`
	return scanUser(r.db.QueryRowContext(ctx, query, id))
}

func scanUser(row *sql.Row) (*model.User, error) {
	var user model.User
	err := row.Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.Password,
		&user.Image,
		&user.IsActive,
		&user.PasswordChangedAt,
		&user.StartDate,
		&user.PeriodStartDay,
		&user.Locale,
		&user.EmailVerifiedAt,
		&user.SessionsRevokedAt,
		&user.TOTPSecret,
		&user.TOTPEnabledAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Update changes only the fields that are not nil and bumps updated_at.
func (r *UserRepository) Update(ctx context.Context, id uuid.UUID, req model.UpdateProfileRequest) (*model.User, error) {
	query := `
	UPDATE users
	SET name = COALESCE($2, name), img_url = COALESCE($3, img_url),
	    period_start_day = COALESCE($4, period_start_day),
	    locale = CASE WHEN $5::text IS NULL THEN locale WHEN $5 = 'auto' THEN NULL ELSE $5 END,
	    updated_at = now()
	WHERE id = $1
	RETURNING ` + userColumns
	return scanUser(r.db.QueryRowContext(ctx, query, id, req.Name, req.Image, req.PeriodStartDay, req.Locale))
}

func (r *UserRepository) UpdatePassword(ctx context.Context, id uuid.UUID, hash string, changedAt time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET password = $2, password_changed_at = $3, updated_at = now() WHERE id = $1`, id, hash, changedAt)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *UserRepository) SetEmailVerified(ctx context.Context, id uuid.UUID) error {
	return expectOne(r.db.ExecContext(ctx,
		`UPDATE users SET email_verified_at = COALESCE(email_verified_at, now()) WHERE id = $1`, id))
}

// RevokeTokensBefore makes every access token issued before t invalid at /me.
func (r *UserRepository) RevokeTokensBefore(ctx context.Context, id uuid.UUID, t time.Time) error {
	return expectOne(r.db.ExecContext(ctx, `UPDATE users SET sessions_revoked_at = $2 WHERE id = $1`, id, t))
}

// SetTOTP stores a new encrypted secret (enabledAt nil = waiting for the first code),
// or clears 2FA when secret is nil. lastStep is the step of the code that enabled it.
func (r *UserRepository) SetTOTP(ctx context.Context, id uuid.UUID, secret []byte, enabledAt *time.Time, lastStep int64) error {
	return expectOne(r.db.ExecContext(ctx,
		`UPDATE users SET totp_secret = $2, totp_enabled_at = $3, totp_last_step = $4 WHERE id = $1`, id, secret, enabledAt, lastStep))
}

// UseTOTPStep records that the code for this time step was used. It returns false when this
// step or a later one was already used, so a code cannot be replayed.
func (r *UserRepository) UseTOTPStep(ctx context.Context, id uuid.UUID, step int64) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE users SET totp_last_step = $2 WHERE id = $1 AND totp_last_step < $2`, id, step)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
