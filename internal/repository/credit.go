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
	ErrNotACard        = errors.New("card not found")
	ErrPlanNotFound    = errors.New("installment plan not found")
	ErrAlreadyPaid     = errors.New("installment already paid")
	ErrPlanHasPayments = errors.New("installment plan already has payments")
)

// NextCode returns the lowest free account code in [from, to].
func (r *LedgerRepository) NextCode(ctx context.Context, userID uuid.UUID, from, to int) (string, error) {
	var code sql.NullString
	err := r.db.QueryRowContext(ctx, `
	SELECT min(n)::text FROM generate_series($2::int, $3::int) n
	WHERE NOT EXISTS (SELECT 1 FROM accounts WHERE user_id = $1 AND code = n::text)`, userID, from, to).Scan(&code)
	if err != nil {
		return "", err
	}
	if !code.Valid {
		return "", ErrAccountCodeTaken
	}
	return code.String, nil
}

// insertAccount stores an account with an ID chosen by the caller, so an entry built before
// the insert can already point at it.
func insertAccount(ctx context.Context, tx *sql.Tx, userID uuid.UUID, a *model.Account) error {
	_, err := tx.ExecContext(ctx, `
	INSERT INTO accounts (id, user_id, code, name, type, category, normal_balance, icon, color, kind)
	VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), NULLIF($9, ''), $10)`,
		a.ID, userID, a.Code, a.Name, a.Type, a.Category, a.NormalBalance, a.Icon, a.Color, a.Kind)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrAccountCodeTaken
	}
	return err
}

func (r *LedgerRepository) CreateCard(ctx context.Context, userID uuid.UUID, c *model.Card) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertAccount(ctx, tx, userID, &c.Account); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
	INSERT INTO cards (account_id, statement_day, due_day, credit_limit, min_payment_pct) VALUES ($1, $2, $3, $4, $5)`,
		c.Account.ID, c.StatementDay, c.DueDay, c.CreditLimit, c.MinPaymentPct); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *LedgerRepository) UpdateCard(ctx context.Context, userID, accountID uuid.UUID, s model.CardSettings) error {
	res, err := r.db.ExecContext(ctx, `
	UPDATE cards SET statement_day = $3, due_day = $4, credit_limit = $5, min_payment_pct = $6
	WHERE account_id = $1 AND account_id IN (SELECT id FROM accounts WHERE user_id = $2)`,
		accountID, userID, s.StatementDay, s.DueDay, s.CreditLimit, s.MinPaymentPct)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotACard
	}
	return nil
}

func (r *LedgerRepository) Cards(ctx context.Context, userID uuid.UUID) ([]model.Card, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT a.id, a.code, a.name, a.type, a.category, a.normal_balance, a.is_active, COALESCE(a.icon, ''), COALESCE(a.color, ''), a.kind,
	       c.statement_day, c.due_day, c.credit_limit, c.min_payment_pct
	FROM cards c JOIN accounts a ON a.id = c.account_id
	WHERE a.user_id = $1 ORDER BY a.code`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Card{}
	for rows.Next() {
		var c model.Card
		a := &c.Account
		if err := rows.Scan(&a.ID, &a.Code, &a.Name, &a.Type, &a.Category, &a.NormalBalance, &a.IsActive, &a.Icon, &a.Color, &a.Kind,
			&c.StatementDay, &c.DueDay, &c.CreditLimit, &c.MinPaymentPct); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreatePlan stores the loan account, the plan and (optionally) the entry for the money
// received, in one transaction.
func (r *LedgerRepository) CreatePlan(ctx context.Context, userID uuid.UUID, p *model.Plan, borrowing *model.Entry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockUser(ctx, tx, userID); err != nil {
		return err
	}
	if err := insertAccount(ctx, tx, userID, &p.Account); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `
	INSERT INTO installment_plans (user_id, account_id, interest_account_id, principal, annual_rate_bp, tenor_months, method, first_due_date)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		userID, p.Account.ID, p.InterestAccountID, p.Principal, p.AnnualRateBP, p.TenorMonths, p.Method, p.FirstDueDate).Scan(&p.ID); err != nil {
		return err
	}
	if borrowing != nil {
		if err := insertEntry(ctx, tx, userID, borrowing); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Plans returns the user's plans with their paid installments; only selects one plan when id is set.
func (r *LedgerRepository) Plans(ctx context.Context, userID uuid.UUID, id *uuid.UUID) ([]model.Plan, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT p.id, p.interest_account_id, p.principal, p.annual_rate_bp, p.tenor_months, p.method, p.first_due_date,
	       a.id, a.code, a.name, a.type, a.category, a.normal_balance, a.is_active, COALESCE(a.icon, ''), COALESCE(a.color, ''), a.kind,
	       COALESCE((SELECT json_agg(json_build_object('seq', ip.seq, 'entry_id', ip.entry_id, 'entry_no', e.entry_no) ORDER BY ip.seq)
	                 FROM installment_payments ip JOIN journal_entries e ON e.id = ip.entry_id WHERE ip.plan_id = p.id), '[]')
	FROM installment_plans p JOIN accounts a ON a.id = p.account_id
	WHERE p.user_id = $1 AND ($2::uuid IS NULL OR p.id = $2)
	ORDER BY a.code`, userID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Plan{}
	for rows.Next() {
		var p model.Plan
		var first time.Time
		var paid []byte
		a := &p.Account
		if err := rows.Scan(&p.ID, &p.InterestAccountID, &p.Principal, &p.AnnualRateBP, &p.TenorMonths, &p.Method, &first,
			&a.ID, &a.Code, &a.Name, &a.Type, &a.Category, &a.NormalBalance, &a.IsActive, &a.Icon, &a.Color, &a.Kind, &paid); err != nil {
			return nil, err
		}
		p.FirstDueDate = first.Format(time.DateOnly)
		if err := unmarshalAll(paid, &p.Paid); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PayInstallment posts the payment entry and marks installment seq paid, atomically.
// The primary key (plan_id, seq) stops a second payment of the same installment.
func (r *LedgerRepository) PayInstallment(ctx context.Context, userID, planID uuid.UUID, seq int, e *model.Entry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockUser(ctx, tx, userID); err != nil {
		return err
	}
	if err := insertEntry(ctx, tx, userID, e); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO installment_payments (plan_id, seq, entry_id) VALUES ($1, $2, $3)`, planID, seq, e.ID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrAlreadyPaid
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// DeletePlan removes a plan that has no payments and deactivates its loan account
// (the account may still carry the borrowing entry or an opening balance).
func (r *LedgerRepository) DeletePlan(ctx context.Context, userID, planID uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var accountID uuid.UUID
	var paid bool
	err = tx.QueryRowContext(ctx, `
	SELECT account_id, EXISTS (SELECT 1 FROM installment_payments WHERE plan_id = p.id)
	FROM installment_plans p WHERE id = $1 AND user_id = $2 FOR UPDATE`, planID, userID).Scan(&accountID, &paid)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPlanNotFound
	}
	if err != nil {
		return err
	}
	if paid {
		return ErrPlanHasPayments
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM installment_plans WHERE id = $1`, planID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE accounts SET is_active = false, updated_at = now() WHERE id = $1`, accountID); err != nil {
		return err
	}
	return tx.Commit()
}
