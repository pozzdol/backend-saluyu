package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
)

// Movements sums debits and credits per account and entry kind for entries dated in [from, to),
// ordered by account code. byPeriodDate dates entries by COALESCE(period_date, entry_date)
// (period reports); otherwise by entry_date (balances: when the money really moved).
// A zero from means since the beginning.
func (r *LedgerRepository) Movements(ctx context.Context, userID uuid.UUID, from, to time.Time, byPeriodDate bool) ([]model.Movement, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT a.id, a.code, a.name, a.type, a.category, e.kind, SUM(l.debit), SUM(l.credit)
	FROM journal_lines l
	JOIN journal_entries e ON e.id = l.entry_id
	JOIN accounts a ON a.id = l.account_id
	WHERE e.user_id = $1
	  AND daterange($2::date, $3::date) -- half-open [from, to)
	      @> CASE WHEN $4::bool THEN COALESCE(e.period_date, e.entry_date) ELSE e.entry_date END
	GROUP BY a.id, e.kind
	ORDER BY a.code, e.kind`, userID, from, to, byPeriodDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Movement
	for rows.Next() {
		var m model.Movement
		if err := rows.Scan(&m.AccountID, &m.Code, &m.Name, &m.Type, &m.Category, &m.Kind, &m.Debit, &m.Credit); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// EffectiveBudgets returns, per account, the latest budget set at or before periodStart:
// a budget carries forward to later periods without being copied.
func (r *LedgerRepository) EffectiveBudgets(ctx context.Context, userID uuid.UUID, periodStart time.Time) ([]model.Budget, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT DISTINCT ON (b.account_id) b.account_id, a.name, b.period_start, b.amount
	FROM budgets b JOIN accounts a ON a.id = b.account_id
	WHERE b.user_id = $1 AND b.period_start <= $2
	ORDER BY b.account_id, b.period_start DESC`, userID, periodStart)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Budget
	for rows.Next() {
		var b model.Budget
		if err := rows.Scan(&b.AccountID, &b.Name, &b.PeriodStart, &b.Amount); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// UpsertBudget sets the budget starting at periodStart. The caller checks the account belongs to userID.
func (r *LedgerRepository) UpsertBudget(ctx context.Context, userID, accountID uuid.UUID, periodStart time.Time, amount int64) error {
	_, err := r.db.ExecContext(ctx, `
	INSERT INTO budgets (user_id, account_id, period_start, amount) VALUES ($1, $2, $3, $4)
	ON CONFLICT (user_id, account_id, period_start) DO UPDATE SET amount = EXCLUDED.amount, updated_at = now()`,
		userID, accountID, periodStart, amount)
	return err
}
