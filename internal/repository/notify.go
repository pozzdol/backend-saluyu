package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
)

type NotifyRepository struct {
	db *sql.DB
}

func NewNotifyRepository(db *sql.DB) *NotifyRepository { return &NotifyRepository{db: db} }

const prefColumns = `COALESCE(p.due_bills, TRUE), COALESCE(p.budget, TRUE), COALESCE(p.weekly, TRUE), COALESCE(p.inactive, TRUE)`

func (r *NotifyRepository) Prefs(ctx context.Context, userID uuid.UUID) (model.NotifyPrefs, error) {
	var p model.NotifyPrefs
	err := r.db.QueryRowContext(ctx, `SELECT `+prefColumns+` FROM users u LEFT JOIN notification_prefs p ON p.user_id = u.id WHERE u.id = $1`, userID).
		Scan(&p.DueBills, &p.Budget, &p.Weekly, &p.Inactive)
	return p, err
}

func (r *NotifyRepository) SetPrefs(ctx context.Context, userID uuid.UUID, p model.NotifyPrefs) error {
	_, err := r.db.ExecContext(ctx, `
	INSERT INTO notification_prefs (user_id, due_bills, budget, weekly, inactive) VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (user_id) DO UPDATE SET due_bills = $2, budget = $3, weekly = $4, inactive = $5, updated_at = now()`,
		userID, p.DueBills, p.Budget, p.Weekly, p.Inactive)
	return err
}

// Recipients are the active users whose email is verified.
func (r *NotifyRepository) Recipients(ctx context.Context) ([]model.NotifyUser, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT u.id, u.name, u.email, u.locale, `+prefColumns+`
	FROM users u LEFT JOIN notification_prefs p ON p.user_id = u.id
	WHERE u.is_active AND u.email_verified_at IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.NotifyUser
	for rows.Next() {
		var u model.NotifyUser
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Locale, &u.Prefs.DueBills, &u.Prefs.Budget, &u.Prefs.Weekly, &u.Prefs.Inactive); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Claim records a reminder as sent; false means it was already sent, so the caller skips it.
func (r *NotifyRepository) Claim(ctx context.Context, userID uuid.UUID, kind, ref string, on time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO notification_log (user_id, kind, ref, sent_on) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
		userID, kind, ref, on)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// Unclaim forgets a claim whose email failed to send, so the next run tries again.
func (r *NotifyRepository) Unclaim(ctx context.Context, userID uuid.UUID, kind, ref string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM notification_log WHERE user_id = $1 AND kind = $2 AND ref = $3`, userID, kind, ref)
	return err
}
