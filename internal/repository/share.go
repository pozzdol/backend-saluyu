package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pozzdol/backend-saluyu/internal/model"
)

var (
	// ErrNotFound is returned for anything the caller may not see, so a 404 never tells
	// whether a group, invite or request exists for someone else.
	ErrNotFound       = errors.New("not found")
	ErrLoanNotPending = errors.New("this request was already answered")
	ErrEntryLocked    = errors.New("this entry is mirrored in another member's book and cannot be deleted")
	ErrAlreadyLinked  = errors.New("another contact is already linked to this member")
)

func (r *LedgerRepository) CreateGroup(ctx context.Context, userID uuid.UUID, name string) (uuid.UUID, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback()
	var id uuid.UUID
	if err := tx.QueryRowContext(ctx, `INSERT INTO groups (name, created_by) VALUES ($1, $2) RETURNING id`, name, userID).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)`, id, userID); err != nil {
		return uuid.Nil, err
	}
	return id, tx.Commit()
}

// ListGroups returns the groups userID is an active member of, with active members and pending invites.
func (r *LedgerRepository) ListGroups(ctx context.Context, userID uuid.UUID) ([]model.Group, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT g.id, g.name,
	       (SELECT json_agg(json_build_object('id', u.id, 'name', u.name, 'email', u.email, 'img_url', u.img_url) ORDER BY u.name)
	        FROM group_members m JOIN users u ON u.id = m.user_id
	        WHERE m.group_id = g.id AND m.status = 'active'),
	       COALESCE((SELECT json_agg(json_build_object('id', i.id, 'email', i.email, 'created_at', i.created_at) ORDER BY i.created_at)
	        FROM group_invites i WHERE i.group_id = g.id), '[]')
	FROM groups g JOIN group_members me ON me.group_id = g.id
	WHERE me.user_id = $1 AND me.status = 'active'
	ORDER BY g.created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.Group{}
	for rows.Next() {
		var g model.Group
		var members, invites []byte
		if err := rows.Scan(&g.ID, &g.Name, &members, &invites); err != nil {
			return nil, err
		}
		if err := unmarshalAll(members, &g.Members, invites, &g.PendingInvites); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *LedgerRepository) IsActiveMember(ctx context.Context, groupID, userID uuid.UUID) (bool, error) {
	var ok bool
	err := r.db.QueryRowContext(ctx, `
	SELECT EXISTS (SELECT 1 FROM group_members WHERE group_id = $1 AND user_id = $2 AND status = 'active')`,
		groupID, userID).Scan(&ok)
	return ok, err
}

func (r *LedgerRepository) ActiveMembers(ctx context.Context, groupID uuid.UUID) ([]model.Member, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT u.id, u.name, u.email, u.img_url
	FROM group_members m JOIN users u ON u.id = m.user_id
	WHERE m.group_id = $1 AND m.status = 'active'
	ORDER BY u.name`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Member{}
	for rows.Next() {
		var m model.Member
		if err := rows.Scan(&m.ID, &m.Name, &m.Email, &m.Image); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SharesActiveGroup is the one access rule of phase 5: both users are active in some group.
func (r *LedgerRepository) SharesActiveGroup(ctx context.Context, a, b uuid.UUID) (bool, error) {
	var ok bool
	err := r.db.QueryRowContext(ctx, `
	SELECT EXISTS (
	    SELECT 1 FROM group_members x JOIN group_members y ON x.group_id = y.group_id
	    WHERE x.user_id = $1 AND y.user_id = $2 AND x.status = 'active' AND y.status = 'active')`, a, b).Scan(&ok)
	return ok, err
}

// Invite stores an invite for an email. Inviting the same email twice is a no-op.
// The email is never looked up in users, so the response cannot reveal who is registered.
func (r *LedgerRepository) Invite(ctx context.Context, groupID, inviter uuid.UUID, email string) (*model.PendingInvite, error) {
	inv := model.PendingInvite{Email: email}
	err := r.db.QueryRowContext(ctx, `
	INSERT INTO group_invites (group_id, email, invited_by) VALUES ($1, $2, $3)
	ON CONFLICT (group_id, email) DO UPDATE SET email = EXCLUDED.email
	RETURNING id, created_at`, groupID, email, inviter).Scan(&inv.ID, &inv.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *LedgerRepository) CancelInvite(ctx context.Context, groupID, inviteID uuid.UUID) error {
	return expectOne(r.db.ExecContext(ctx, `DELETE FROM group_invites WHERE id = $1 AND group_id = $2`, inviteID, groupID))
}

func (r *LedgerRepository) InvitesForEmail(ctx context.Context, email string) ([]model.InboxInvite, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT i.id, g.id, g.name, u.name, i.created_at
	FROM group_invites i JOIN groups g ON g.id = i.group_id JOIN users u ON u.id = i.invited_by
	WHERE i.email = $1
	ORDER BY i.created_at`, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.InboxInvite{}
	for rows.Next() {
		var i model.InboxInvite
		if err := rows.Scan(&i.ID, &i.GroupID, &i.GroupName, &i.InvitedByName, &i.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// RespondInvite consumes an invite addressed to email; accept makes userID an active member.
func (r *LedgerRepository) RespondInvite(ctx context.Context, inviteID, userID uuid.UUID, email string, accept bool) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var groupID uuid.UUID
	err = tx.QueryRowContext(ctx, `DELETE FROM group_invites WHERE id = $1 AND email = $2 RETURNING group_id`, inviteID, email).Scan(&groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if accept {
		_, err = tx.ExecContext(ctx, `
		INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)
		ON CONFLICT (group_id, user_id) DO UPDATE SET status = 'active', joined_at = now(), left_at = NULL`, groupID, userID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *LedgerRepository) LeaveGroup(ctx context.Context, groupID, userID uuid.UUID) error {
	return expectOne(r.db.ExecContext(ctx, `
	UPDATE group_members SET status = 'left', left_at = now()
	WHERE group_id = $1 AND user_id = $2 AND status = 'active'`, groupID, userID))
}

func (r *LedgerRepository) LinkContact(ctx context.Context, userID, contactID uuid.UUID, linked *uuid.UUID) error {
	err := expectOne(r.db.ExecContext(ctx, `UPDATE contacts SET linked_user_id = $3 WHERE id = $1 AND user_id = $2`, contactID, userID, linked))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrAlreadyLinked
	}
	if errors.Is(err, ErrNotFound) {
		return ErrContactNotFound
	}
	return err
}

// LinkedUser returns the user a contact is linked to, if any.
func (r *LedgerRepository) LinkedUser(ctx context.Context, userID, contactID uuid.UUID) (*uuid.UUID, error) {
	var linked *uuid.UUID
	err := r.db.QueryRowContext(ctx, `SELECT linked_user_id FROM contacts WHERE id = $1 AND user_id = $2`, contactID, userID).Scan(&linked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrContactNotFound
	}
	return linked, err
}

// createLoanRequest runs inside the entry's transaction: a loan or repayment on a contact
// linked to another user asks that user to mirror it. Amount is the size of the change.
func createLoanRequest(ctx context.Context, tx *sql.Tx, e *model.Entry) error {
	if e.Kind != "loan" && e.Kind != "repayment" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
	INSERT INTO loan_requests (from_entry_id, from_user_id, to_user_id, amount)
	SELECT e.id, e.user_id, c.linked_user_id, ABS(SUM(l.debit - l.credit))
	FROM journal_entries e
	JOIN journal_lines l ON l.entry_id = e.id
	JOIN contacts c ON c.id = l.contact_id
	WHERE e.id = $1 AND c.linked_user_id IS NOT NULL
	GROUP BY e.id, e.user_id, c.linked_user_id
	HAVING SUM(l.debit - l.credit) <> 0`, e.ID)
	return err
}

func (r *LedgerRepository) PendingLoans(ctx context.Context, userID uuid.UUID) ([]model.InboxLoan, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT q.id, u.name, e.kind, q.amount, e.entry_date, q.created_at,
	       (SELECT SUM(l.debit - l.credit) FROM journal_lines l WHERE l.entry_id = e.id AND l.contact_id IS NOT NULL)
	FROM loan_requests q
	JOIN users u ON u.id = q.from_user_id
	JOIN journal_entries e ON e.id = q.from_entry_id
	WHERE q.to_user_id = $1 AND q.status = 'pending'
	ORDER BY q.created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.InboxLoan{}
	for rows.Next() {
		var l model.InboxLoan
		var date time.Time
		if err := rows.Scan(&l.ID, &l.FromName, &l.Kind, &l.Amount, &date, &l.CreatedAt, &l.Delta); err != nil {
			return nil, err
		}
		l.Date = date.Format(time.DateOnly)
		out = append(out, l)
	}
	return out, rows.Err()
}

// LoanSource reads a request addressed to userID together with the sender's contact lines.
func (r *LedgerRepository) LoanSource(ctx context.Context, requestID, userID uuid.UUID) (*model.LoanSource, error) {
	var s model.LoanSource
	var lines []byte
	err := r.db.QueryRowContext(ctx, `
	SELECT q.status, q.to_entry_id, q.from_user_id, u.name, u.email, e.kind, e.entry_date,
	       (SELECT json_agg(json_build_object('account_id', l.account_id, 'code', a.code, 'debit', l.debit, 'credit', l.credit) ORDER BY l.id)
	        FROM journal_lines l JOIN accounts a ON a.id = l.account_id JOIN contacts c ON c.id = l.contact_id
	        WHERE l.entry_id = e.id AND c.linked_user_id = q.to_user_id)
	FROM loan_requests q
	JOIN users u ON u.id = q.from_user_id
	JOIN journal_entries e ON e.id = q.from_entry_id
	WHERE q.id = $1 AND q.to_user_id = $2`, requestID, userID).
		Scan(&s.Status, &s.ToEntryID, &s.FromUserID, &s.FromName, &s.FromEmail, &s.Kind, &s.Date, &lines)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := unmarshalAll(lines, &s.Lines); err != nil {
		return nil, err
	}
	return &s, nil
}

// ContactForUser finds the approver's contact linked to the sender, creating it if needed.
func (r *LedgerRepository) ContactForUser(ctx context.Context, userID, linked uuid.UUID, name string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRowContext(ctx, `SELECT id FROM contacts WHERE user_id = $1 AND linked_user_id = $2`, userID, linked).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, err
	}
	err = r.db.QueryRowContext(ctx, `INSERT INTO contacts (user_id, name, linked_user_id) VALUES ($1, $2, $3) RETURNING id`,
		userID, name, linked).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		// Same name already used, or a concurrent approve created it first: read it back.
		err = r.db.QueryRowContext(ctx, `SELECT id FROM contacts WHERE user_id = $1 AND linked_user_id = $2`, userID, linked).Scan(&id)
	}
	return id, err
}

// ApproveLoan writes the mirror entry and marks the request approved in one transaction.
// The request row is locked first, so a double click or a retry gets the same entry back
// instead of posting twice.
func (r *LedgerRepository) ApproveLoan(ctx context.Context, requestID, userID uuid.UUID, e *model.Entry) (uuid.UUID, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback()

	var status string
	var existing *uuid.UUID
	err = tx.QueryRowContext(ctx, `SELECT status, to_entry_id FROM loan_requests WHERE id = $1 AND to_user_id = $2 FOR UPDATE`,
		requestID, userID).Scan(&status, &existing)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	switch status {
	case "approved":
		return *existing, nil
	case "rejected":
		return uuid.Nil, ErrLoanNotPending
	}

	if err := lockUser(ctx, tx, userID); err != nil {
		return uuid.Nil, err
	}
	if err := insertEntry(ctx, tx, userID, e); err != nil {
		return uuid.Nil, err
	}
	if _, err := tx.ExecContext(ctx, `
	UPDATE loan_requests SET status = 'approved', to_entry_id = $2, responded_at = now() WHERE id = $1`, requestID, e.ID); err != nil {
		return uuid.Nil, err
	}
	return e.ID, tx.Commit()
}

func (r *LedgerRepository) RejectLoan(ctx context.Context, requestID, userID uuid.UUID) error {
	err := expectOne(r.db.ExecContext(ctx, `
	UPDATE loan_requests SET status = 'rejected', responded_at = now()
	WHERE id = $1 AND to_user_id = $2 AND status = 'pending'`, requestID, userID))
	if errors.Is(err, ErrNotFound) {
		// Distinguish "answered already" from "not yours / does not exist".
		var exists bool
		if e := r.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM loan_requests WHERE id = $1 AND to_user_id = $2)`,
			requestID, userID).Scan(&exists); e == nil && exists {
			return ErrLoanNotPending
		}
	}
	return err
}

// expectOne turns "no row changed" into ErrNotFound.
func expectOne(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// unmarshalAll decodes (json, target) pairs; a NULL json (nil bytes) leaves the target as is.
func unmarshalAll(pairs ...any) error {
	for i := 0; i+1 < len(pairs); i += 2 {
		if b := pairs[i].([]byte); len(b) > 0 {
			if err := json.Unmarshal(b, pairs[i+1]); err != nil {
				return err
			}
		}
	}
	return nil
}
