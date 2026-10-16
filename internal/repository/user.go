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

const userColumns = `id, name, email, password, img_url, is_active, password_changed_at, start_date, period_start_day, created_at, updated_at`

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
	    period_start_day = COALESCE($4, period_start_day), updated_at = now()
	WHERE id = $1
	RETURNING ` + userColumns
	return scanUser(r.db.QueryRowContext(ctx, query, id, req.Name, req.Image, req.PeriodStartDay))
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
