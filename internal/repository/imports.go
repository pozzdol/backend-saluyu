package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/statement"
)

var ErrRuleMissing = errors.New("category rule not found")

func (r *LedgerRepository) CategoryRules(ctx context.Context, userID uuid.UUID) ([]model.CategoryRule, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT id, keyword, min_amount, max_amount, account_id FROM category_rules WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.CategoryRule{}
	for rows.Next() {
		var c model.CategoryRule
		if err := rows.Scan(&c.ID, &c.Keyword, &c.MinAmount, &c.MaxAmount, &c.AccountID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *LedgerRepository) CreateCategoryRule(ctx context.Context, userID uuid.UUID, c *model.CategoryRule) error {
	return r.db.QueryRowContext(ctx, `
	INSERT INTO category_rules (user_id, keyword, min_amount, max_amount, account_id) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		userID, c.Keyword, c.MinAmount, c.MaxAmount, c.AccountID).Scan(&c.ID)
}

func (r *LedgerRepository) DeleteCategoryRule(ctx context.Context, userID, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM category_rules WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrRuleMissing
	}
	return nil
}

func (r *LedgerRepository) ImportProfile(ctx context.Context, accountID uuid.UUID) (*statement.Mapping, error) {
	var raw []byte
	err := r.db.QueryRowContext(ctx, `SELECT mapping FROM import_profiles WHERE account_id = $1`, accountID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m statement.Mapping
	return &m, json.Unmarshal(raw, &m)
}

// ImportedHashes returns which of the hashes were imported before, with their entries.
// Hashes are hex strings, the way the preview sends them.
func (r *LedgerRepository) ImportedHashes(ctx context.Context, accountID uuid.UUID, hashes []string) (map[string]uuid.UUID, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT encode(row_hash, 'hex'), entry_id FROM imported_rows WHERE account_id = $1 AND encode(row_hash, 'hex') = ANY($2::text[])`, accountID, hashes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]uuid.UUID{}
	for rows.Next() {
		var h string
		var id uuid.UUID
		if err := rows.Scan(&h, &id); err != nil {
			return nil, err
		}
		out[h] = id
	}
	return out, rows.Err()
}

// MatchCandidates lists the account's entries in [from, to] that no statement row is linked to.
func (r *LedgerRepository) MatchCandidates(ctx context.Context, userID, accountID uuid.UUID, from, to time.Time) ([]model.MatchCandidate, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT e.id, e.entry_no, e.entry_date, SUM(l.debit - l.credit)
	FROM journal_entries e JOIN journal_lines l ON l.entry_id = e.id
	WHERE e.user_id = $1 AND l.account_id = $2 AND e.entry_date BETWEEN $3 AND $4
	  AND NOT EXISTS (SELECT 1 FROM imported_rows i WHERE i.entry_id = e.id AND i.account_id = $2)
	GROUP BY e.id ORDER BY e.entry_date, e.entry_no`, userID, accountID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.MatchCandidate{}
	for rows.Next() {
		var c model.MatchCandidate
		var d time.Time
		if err := rows.Scan(&c.EntryID, &c.EntryNo, &d, &c.Amount); err != nil {
			return nil, err
		}
		c.Date = d.Format(time.DateOnly)
		out = append(out, c)
	}
	return out, rows.Err()
}

// CommitImport posts new rows and links matched ones in one transaction. A row whose hash
// is already stored (another import got there first) is skipped, not posted twice.
func (r *LedgerRepository) CommitImport(ctx context.Context, userID, accountID uuid.UUID, rows []model.ImportedRow, mapping statement.Mapping) (posted, matched, skipped int, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, 0, err
	}
	defer tx.Rollback()
	if err := lockUser(ctx, tx, userID); err != nil {
		return 0, 0, 0, err
	}
	for _, row := range rows {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM imported_rows WHERE account_id = $1 AND row_hash = $2)`,
			accountID, row.Hash).Scan(&exists); err != nil {
			return 0, 0, 0, err
		}
		if exists {
			skipped++
			continue
		}
		entryID := row.EntryID
		if row.Entry != nil {
			if err := insertEntry(ctx, tx, userID, row.Entry); err != nil {
				return 0, 0, 0, err
			}
			entryID = &row.Entry.ID
			posted++
		} else {
			// The linked entry must belong to this user and touch this account.
			var ok bool
			if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM journal_entries e JOIN journal_lines l ON l.entry_id = e.id
			               WHERE e.id = $1 AND e.user_id = $2 AND l.account_id = $3)`, entryID, userID, accountID).Scan(&ok); err != nil {
				return 0, 0, 0, err
			}
			if !ok {
				return 0, 0, 0, ErrEntryNotFound
			}
			matched++
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO imported_rows (account_id, row_hash, entry_id) VALUES ($1, $2, $3)`,
			accountID, row.Hash, entryID); err != nil {
			return 0, 0, 0, err
		}
	}
	raw, _ := json.Marshal(mapping)
	if _, err := tx.ExecContext(ctx, `
	INSERT INTO import_profiles (account_id, mapping) VALUES ($1, $2)
	ON CONFLICT (account_id) DO UPDATE SET mapping = EXCLUDED.mapping`, accountID, raw); err != nil {
		return 0, 0, 0, err
	}
	return posted, matched, skipped, tx.Commit()
}
