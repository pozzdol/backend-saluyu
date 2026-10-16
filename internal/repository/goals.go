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
	ErrGoalNotFound   = errors.New("goal not found")
	ErrInvalidInvite  = errors.New("invite link is invalid, used or expired")
	ErrAlreadyInGroup = errors.New("already a member of this group")
)

func (r *LedgerRepository) Goals(ctx context.Context, userID uuid.UUID) ([]model.Goal, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT id, name, target, deadline, account_id FROM goals WHERE user_id = $1 ORDER BY deadline NULLS LAST, name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Goal{}
	for rows.Next() {
		g := model.Goal{UserID: userID}
		var deadline *time.Time
		if err := rows.Scan(&g.ID, &g.Name, &g.Target, &deadline, &g.AccountID); err != nil {
			return nil, err
		}
		g.Deadline = model.DateString(deadline)
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *LedgerRepository) SaveGoal(ctx context.Context, g *model.Goal) error {
	if g.ID == uuid.Nil {
		return r.db.QueryRowContext(ctx, `
		INSERT INTO goals (user_id, name, target, deadline, account_id) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			g.UserID, g.Name, g.Target, g.Deadline, g.AccountID).Scan(&g.ID)
	}
	res, err := r.db.ExecContext(ctx, `
	UPDATE goals SET name = $3, target = $4, deadline = $5, account_id = $6 WHERE id = $1 AND user_id = $2`,
		g.ID, g.UserID, g.Name, g.Target, g.Deadline, g.AccountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrGoalNotFound
	}
	return nil
}

func (r *LedgerRepository) DeleteGoal(ctx context.Context, userID, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM goals WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrGoalNotFound
	}
	return nil
}

func (r *LedgerRepository) CreateInviteLink(ctx context.Context, groupID, by uuid.UUID, hash []byte, expires time.Time) error {
	_, err := r.db.ExecContext(ctx, `
	INSERT INTO group_invite_links (token_hash, group_id, created_by, expires_at) VALUES ($1, $2, $3, $4)`, hash, groupID, by, expires)
	return err
}

// AcceptInviteLink spends a link and adds the user to its group, in one transaction.
func (r *LedgerRepository) AcceptInviteLink(ctx context.Context, hash []byte, userID uuid.UUID) (uuid.UUID, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback()
	var groupID uuid.UUID
	err = tx.QueryRowContext(ctx, `
	SELECT group_id FROM group_invite_links WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now() FOR UPDATE`, hash).Scan(&groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, ErrInvalidInvite
	}
	if err != nil {
		return uuid.Nil, err
	}
	var member bool
	if err := tx.QueryRowContext(ctx, `
	SELECT EXISTS (SELECT 1 FROM group_members WHERE group_id = $1 AND user_id = $2 AND status = 'active')`, groupID, userID).Scan(&member); err != nil {
		return uuid.Nil, err
	}
	if member {
		return uuid.Nil, ErrAlreadyInGroup // the link stays usable for the person it was meant for
	}
	if _, err := tx.ExecContext(ctx, `
	INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)
	ON CONFLICT (group_id, user_id) DO UPDATE SET status = 'active', joined_at = now(), left_at = NULL`, groupID, userID); err != nil {
		return uuid.Nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE group_invite_links SET used_by = $2, used_at = now() WHERE token_hash = $1`, hash, userID); err != nil {
		return uuid.Nil, err
	}
	return groupID, tx.Commit()
}
