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
	ErrRuleNotFound      = errors.New("recurring rule not found")
	ErrOccurrenceHandled = errors.New("this occurrence was already posted or skipped")
)

const ruleColumns = `id, user_id, name, kind, from_account_id, to_account_id, amount, frequency, day, mode, next_date, end_date, last_error`

func scanRules(rows *sql.Rows, err error) ([]model.RecurringRule, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.RecurringRule{}
	for rows.Next() {
		var r model.RecurringRule
		var next time.Time
		var end *time.Time
		if err := rows.Scan(&r.ID, &r.UserID, &r.Name, &r.Kind, &r.FromAccountID, &r.ToAccountID, &r.Amount, &r.Frequency,
			&r.Day, &r.Mode, &next, &end, &r.LastError); err != nil {
			return nil, err
		}
		r.NextDate, r.EndDate = next.Format(time.DateOnly), model.DateString(end)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (r *LedgerRepository) SaveRule(ctx context.Context, rule *model.RecurringRule) error {
	if rule.ID == uuid.Nil {
		return r.db.QueryRowContext(ctx, `
		INSERT INTO recurring_rules (user_id, name, kind, from_account_id, to_account_id, amount, frequency, day, mode, next_date, end_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
			rule.UserID, rule.Name, rule.Kind, rule.FromAccountID, rule.ToAccountID, rule.Amount, rule.Frequency, rule.Day,
			rule.Mode, rule.NextDate, rule.EndDate).Scan(&rule.ID)
	}
	res, err := r.db.ExecContext(ctx, `
	UPDATE recurring_rules SET name = $3, kind = $4, from_account_id = $5, to_account_id = $6, amount = $7, frequency = $8,
	       day = $9, mode = $10, next_date = $11, end_date = $12, last_error = NULL
	WHERE id = $1 AND user_id = $2`,
		rule.ID, rule.UserID, rule.Name, rule.Kind, rule.FromAccountID, rule.ToAccountID, rule.Amount, rule.Frequency, rule.Day,
		rule.Mode, rule.NextDate, rule.EndDate)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrRuleNotFound
	}
	return nil
}

func (r *LedgerRepository) DeleteRule(ctx context.Context, userID, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM recurring_rules WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrRuleNotFound
	}
	return nil
}

func (r *LedgerRepository) Rules(ctx context.Context, userID uuid.UUID) ([]model.RecurringRule, error) {
	return scanRules(r.db.QueryContext(ctx, `SELECT `+ruleColumns+` FROM recurring_rules WHERE user_id = $1 ORDER BY name`, userID))
}

// DueRules are rules with an occurrence on or before today; userID and mode are optional filters.
func (r *LedgerRepository) DueRules(ctx context.Context, today time.Time, userID *uuid.UUID, mode string) ([]model.RecurringRule, error) {
	return scanRules(r.db.QueryContext(ctx, `
	SELECT `+ruleColumns+` FROM recurring_rules
	WHERE next_date <= $1 AND (end_date IS NULL OR next_date <= end_date)
	  AND ($2::uuid IS NULL OR user_id = $2) AND ($3 = '' OR mode::text = $3)
	ORDER BY next_date`, today, userID, mode))
}

// Handled returns the dates from `from` on that were already posted or skipped.
func (r *LedgerRepository) Handled(ctx context.Context, ruleID uuid.UUID, from time.Time) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT due_date FROM recurring_occurrences WHERE rule_id = $1 AND due_date >= $2`, ruleID, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out[d.Format(time.DateOnly)] = true
	}
	return out, rows.Err()
}

// HandleOccurrence claims one occurrence and posts its entry (nil = skip) in one transaction.
// The claim comes first: if another request or job already holds (rule, date), nothing is posted.
func (r *LedgerRepository) HandleOccurrence(ctx context.Context, userID, ruleID uuid.UUID, due time.Time, e *model.Entry, next time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockUser(ctx, tx, userID); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `
	INSERT INTO recurring_occurrences (rule_id, due_date)
	SELECT id, $3 FROM recurring_rules WHERE id = $1 AND user_id = $2
	ON CONFLICT DO NOTHING`, ruleID, userID, due)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrOccurrenceHandled
	}
	if e != nil {
		if err := insertEntry(ctx, tx, userID, e); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE recurring_occurrences SET entry_id = $3 WHERE rule_id = $1 AND due_date = $2`, ruleID, due, e.ID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE recurring_rules SET next_date = GREATEST(next_date, $2), last_error = NULL WHERE id = $1`, ruleID, next); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *LedgerRepository) SetRuleError(ctx context.Context, ruleID uuid.UUID, msg string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE recurring_rules SET last_error = $2 WHERE id = $1`, ruleID, msg)
	return err
}
