package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
)

// GetEntry loads one of the user's entries with its lines.
func (r *LedgerRepository) GetEntry(ctx context.Context, userID, id uuid.UUID) (*model.Entry, error) {
	var e model.Entry
	var lines []byte
	err := r.db.QueryRowContext(ctx, `
	SELECT e.id, e.entry_no, e.entry_date, e.period_date, e.kind, e.description, e.created_at,
	       (SELECT json_agg(json_build_object(
	                   'account_id', l.account_id, 'code', a.code, 'name', a.name,
	                   'debit', l.debit, 'credit', l.credit,
	                   'contact_id', l.contact_id, 'contact_name', c.name) ORDER BY l.id)
	        FROM journal_lines l JOIN accounts a ON a.id = l.account_id
	        LEFT JOIN contacts c ON c.id = l.contact_id
	        WHERE l.entry_id = e.id)
	FROM journal_entries e WHERE e.id = $1 AND e.user_id = $2`, id, userID).
		Scan(&e.ID, &e.EntryNo, &e.EntryDate, &e.PeriodDate, &e.Kind, &e.Description, &e.CreatedAt, &lines)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrEntryNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := unmarshalAll(lines, &e.Lines); err != nil {
		return nil, err
	}
	e.Date = e.EntryDate.Format(time.DateOnly)
	e.PeriodDateStr = model.DateString(e.PeriodDate)
	return &e, nil
}

// LoanLink tells whether an entry takes part in a shared loan: its request's status, the
// request ID, whether this entry is the sender's original (not the mirror), and the other user.
func (r *LedgerRepository) LoanLink(ctx context.Context, entryID uuid.UUID) (status string, requestID uuid.UUID, sender bool, other uuid.UUID, err error) {
	var from uuid.UUID
	var to uuid.UUID
	err = r.db.QueryRowContext(ctx, `
	SELECT status, id, from_entry_id = $1, from_user_id, to_user_id
	FROM loan_requests WHERE from_entry_id = $1 OR to_entry_id = $1`, entryID).Scan(&status, &requestID, &sender, &from, &to)
	if errors.Is(err, sql.ErrNoRows) {
		return "", uuid.Nil, false, uuid.Nil, nil
	}
	if sender {
		other = to
	} else {
		other = from
	}
	return status, requestID, sender, other, err
}

// ReplaceEntry rewrites an entry in place (same ID and number) and keeps a revision.
// A request to another member that was not approved yet is recreated from the new lines,
// so they are asked about the amount that now stands.
func (r *LedgerRepository) ReplaceEntry(ctx context.Context, userID uuid.UUID, before, after *model.Entry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lockUser(ctx, tx, userID); err != nil {
		return err
	}
	if err := replaceInTx(ctx, tx, userID, before, after); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM loan_requests WHERE from_entry_id = $1 AND status <> 'approved'`, after.ID); err != nil {
		return err
	}
	if err := createLoanRequest(ctx, tx, after); err != nil {
		return err
	}
	return tx.Commit()
}

// replaceInTx stores a revision, then swaps header and lines. author is who made the change.
func replaceInTx(ctx context.Context, tx *sql.Tx, author uuid.UUID, before, after *model.Entry) error {
	b, err := json.Marshal(model.Snapshot(before))
	if err != nil {
		return err
	}
	a, err := json.Marshal(model.Snapshot(after))
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO entry_revisions (entry_id, user_id, before, after) VALUES ($1, $2, $3, $4)`, before.ID, author, b, a); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE journal_entries SET entry_date = $2, period_date = $3, description = $4 WHERE id = $1`,
		before.ID, after.EntryDate, after.PeriodDate, after.Description); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM journal_lines WHERE entry_id = $1`, before.ID); err != nil {
		return err
	}
	after.ID, after.EntryNo, after.CreatedAt = before.ID, before.EntryNo, before.CreatedAt
	return insertLines(ctx, tx, after)
}

func (r *LedgerRepository) Revisions(ctx context.Context, entryID uuid.UUID) ([]model.Revision, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT v.changed_at, u.name, v.before, v.after
	FROM entry_revisions v JOIN users u ON u.id = v.user_id
	WHERE v.entry_id = $1 ORDER BY v.changed_at, v.id`, entryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Revision{}
	for rows.Next() {
		var v model.Revision
		var before, after []byte
		if err := rows.Scan(&v.ChangedAt, &v.ChangedBy, &before, &after); err != nil {
			return nil, err
		}
		if err := unmarshalAll(before, &v.Before, after, &v.After); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ProposeChange asks the other member to accept an edit; a newer edit replaces a pending one.
func (r *LedgerRepository) ProposeChange(ctx context.Context, loanRequestID, userID uuid.UUID, p model.ChangeProposal) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
	UPDATE loan_change_requests SET status = 'cancelled', responded_at = now()
	WHERE loan_request_id = $1 AND status = 'pending'`, loanRequestID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
	INSERT INTO loan_change_requests (loan_request_id, requested_by, proposed) VALUES ($1, $2, $3)`,
		loanRequestID, userID, body); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *LedgerRepository) ChangePending(ctx context.Context, loanRequestID uuid.UUID) (bool, error) {
	var ok bool
	err := r.db.QueryRowContext(ctx, `
	SELECT EXISTS (SELECT 1 FROM loan_change_requests WHERE loan_request_id = $1 AND status = 'pending')`, loanRequestID).Scan(&ok)
	return ok, err
}

// PendingChanges lists edits waiting for userID, with the amounts before and after.
func (r *LedgerRepository) PendingChanges(ctx context.Context, userID uuid.UUID) ([]model.InboxChange, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT c.id, u.name, q.amount, e.entry_date, c.proposed, c.created_at
	FROM loan_change_requests c
	JOIN loan_requests q ON q.id = c.loan_request_id
	JOIN users u ON u.id = q.from_user_id
	JOIN journal_entries e ON e.id = q.from_entry_id
	WHERE q.to_user_id = $1 AND c.status = 'pending'
	ORDER BY c.created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.InboxChange{}
	for rows.Next() {
		var c model.InboxChange
		var date time.Time
		var proposed []byte
		if err := rows.Scan(&c.ID, &c.FromName, &c.Before.Amount, &date, &proposed, &c.CreatedAt); err != nil {
			return nil, err
		}
		var p model.ChangeProposal
		if err := json.Unmarshal(proposed, &p); err != nil {
			return nil, err
		}
		c.Before.Date = date.Format(time.DateOnly)
		c.After = model.AmountDate{Amount: abs(netOf(p.MirrorLines)), Date: p.Entry.Date}
		out = append(out, c)
	}
	return out, rows.Err()
}

func netOf(lines []model.Line) (n int64) {
	for _, l := range lines {
		n += l.Debit - l.Credit
	}
	return n
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// ChangeSource reads a change addressed to userID (the member who mirrored the loan).
func (r *LedgerRepository) ChangeSource(ctx context.Context, changeID, userID uuid.UUID) (*model.ChangeSource, error) {
	var s model.ChangeSource
	var proposed []byte
	err := r.db.QueryRowContext(ctx, `
	SELECT c.status, q.id, q.from_user_id, u.name, q.from_entry_id, q.to_entry_id, c.proposed
	FROM loan_change_requests c
	JOIN loan_requests q ON q.id = c.loan_request_id
	JOIN users u ON u.id = q.from_user_id
	WHERE c.id = $1 AND q.to_user_id = $2`, changeID, userID).
		Scan(&s.Status, &s.LoanRequestID, &s.FromUserID, &s.FromName, &s.FromEntryID, &s.ToEntryID, &proposed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, json.Unmarshal(proposed, &s.Proposal)
}

// ApplyChange rewrites both books in one transaction once the other member approves.
// The change row is locked first, so approving twice applies it once.
func (r *LedgerRepository) ApplyChange(ctx context.Context, changeID, approver, sender uuid.UUID, senderBefore, senderAfter, mirrorBefore, mirrorAfter *model.Entry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var status string
	var loanRequestID uuid.UUID
	err = tx.QueryRowContext(ctx, `SELECT status, loan_request_id FROM loan_change_requests WHERE id = $1 FOR UPDATE`, changeID).Scan(&status, &loanRequestID)
	if err != nil {
		return err
	}
	switch status {
	case "approved":
		return nil
	case "pending":
	default:
		return ErrLoanNotPending
	}

	// Lock both users in a fixed order so two approvals in opposite directions cannot deadlock.
	first, second := sender, approver
	if second.String() < first.String() {
		first, second = second, first
	}
	for _, id := range []uuid.UUID{first, second} {
		if err := lockUser(ctx, tx, id); err != nil {
			return err
		}
	}
	if err := replaceInTx(ctx, tx, sender, senderBefore, senderAfter); err != nil {
		return err
	}
	if err := replaceInTx(ctx, tx, approver, mirrorBefore, mirrorAfter); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE loan_requests SET amount = $2 WHERE id = $1`, loanRequestID, abs(netOf(contactLines(mirrorAfter)))); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
	UPDATE loan_change_requests SET status = 'approved', responded_at = now() WHERE id = $1`, changeID); err != nil {
		return err
	}
	return tx.Commit()
}

func contactLines(e *model.Entry) []model.Line {
	var out []model.Line
	for _, l := range e.Lines {
		if l.ContactID != nil {
			out = append(out, l)
		}
	}
	return out
}

func (r *LedgerRepository) RejectChange(ctx context.Context, changeID, userID uuid.UUID) error {
	err := expectOne(r.db.ExecContext(ctx, `
	UPDATE loan_change_requests c SET status = 'rejected', responded_at = now()
	FROM loan_requests q
	WHERE c.id = $1 AND q.id = c.loan_request_id AND q.to_user_id = $2 AND c.status = 'pending'`, changeID, userID))
	if errors.Is(err, ErrNotFound) {
		var exists bool
		if e := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM loan_change_requests c JOIN loan_requests q ON q.id = c.loan_request_id
		               WHERE c.id = $1 AND q.to_user_id = $2)`, changeID, userID).Scan(&exists); e == nil && exists {
			return ErrLoanNotPending
		}
	}
	return err
}
