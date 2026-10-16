package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pozzdol/backend-saluyu/internal/model"
)

var (
	ErrContactNotFound   = errors.New("contact not found")
	ErrContactNameTaken  = errors.New("a contact with this name already exists")
	ErrContactHasHistory = errors.New("contact has transactions and cannot be deleted")
)

// ListContacts returns contacts with their receivable (1101) and payable (2101) balances.
// only narrows the result to one contact; nil returns all of the user's contacts.
func (r *LedgerRepository) ListContacts(ctx context.Context, userID uuid.UUID, only *uuid.UUID) ([]model.Contact, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT c.id, c.name, c.linked_user_id,
	       COALESCE(SUM(CASE WHEN a.code = '1101' THEN l.debit - l.credit END), 0),
	       COALESCE(SUM(CASE WHEN a.code = '2101' THEN l.credit - l.debit END), 0)
	FROM contacts c
	LEFT JOIN journal_lines l ON l.contact_id = c.id
	LEFT JOIN accounts a ON a.id = l.account_id
	WHERE c.user_id = $1 AND ($2::uuid IS NULL OR c.id = $2)
	GROUP BY c.id
	ORDER BY lower(c.name)`, userID, only)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Contact{}
	for rows.Next() {
		var c model.Contact
		if err := rows.Scan(&c.ID, &c.Name, &c.LinkedUserID, &c.Receivable, &c.Payable); err != nil {
			return nil, err
		}
		c.Net = c.Receivable - c.Payable
		out = append(out, c)
	}
	return out, rows.Err()
}

// ContactIDs returns which of ids belong to userID.
func (r *LedgerRepository) ContactIDs(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	strIDs := make([]string, len(ids))
	for i, id := range ids {
		strIDs[i] = id.String()
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM contacts WHERE user_id = $1 AND id = ANY($2::uuid[])`, userID, strIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]bool{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (r *LedgerRepository) CreateContact(ctx context.Context, userID uuid.UUID, name string) (*model.Contact, error) {
	c := model.Contact{Name: name}
	err := r.db.QueryRowContext(ctx, `INSERT INTO contacts (user_id, name) VALUES ($1, $2) RETURNING id`, userID, name).Scan(&c.ID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return nil, ErrContactNameTaken
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *LedgerRepository) RenameContact(ctx context.Context, userID, id uuid.UUID, name string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE contacts SET name = $3 WHERE id = $1 AND user_id = $2`, id, userID, name)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrContactNameTaken
	}
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrContactNotFound
	}
	return nil
}

// blockedByReference reports a delete stopped by a foreign key: ON DELETE RESTRICT raises
// 23001 (restrict_violation), NO ACTION 23503 (foreign_key_violation).
func blockedByReference(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "23001" || pgErr.Code == "23503")
}

func (r *LedgerRepository) DeleteContact(ctx context.Context, userID, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM contacts WHERE id = $1 AND user_id = $2`, id, userID)
	if blockedByReference(err) { // journal_lines still point at it
		return ErrContactHasHistory
	}
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrContactNotFound
	}
	return nil
}

// ContactHistory returns a contact's receivable and payable lines in date order.
func (r *LedgerRepository) ContactHistory(ctx context.Context, userID, contactID uuid.UUID) ([]model.ContactLine, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT e.id, e.entry_no, e.entry_date, e.kind, e.description, a.code, l.debit, l.credit
	FROM journal_lines l
	JOIN journal_entries e ON e.id = l.entry_id
	JOIN accounts a ON a.id = l.account_id
	WHERE e.user_id = $1 AND l.contact_id = $2
	ORDER BY e.entry_date, e.entry_no, l.id`, userID, contactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.ContactLine{}
	for rows.Next() {
		var l model.ContactLine
		var date time.Time
		if err := rows.Scan(&l.EntryID, &l.EntryNo, &date, &l.Kind, &l.Description, &l.Code, &l.Debit, &l.Credit); err != nil {
			return nil, err
		}
		l.Date = date.Format(time.DateOnly)
		out = append(out, l)
	}
	return out, rows.Err()
}
