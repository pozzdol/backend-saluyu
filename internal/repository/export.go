package repository

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
)

// exports are the tables of a data download. Every column is cast to text so the CSV and
// JSON show exactly what the database holds; money is rupiah with two decimals.
var exports = []struct{ name, query string }{
	{"profile", `
	SELECT name, email, period_start_day::text, COALESCE(start_date::text, '') AS start_date,
	       COALESCE(locale, '') AS locale, created_at::text
	FROM users WHERE id = $1`},
	{"accounts", `
	SELECT code, name, type::text, category, normal_balance::text, is_active::text,
	       COALESCE(icon, '') AS icon, COALESCE(color, '') AS color
	FROM accounts WHERE user_id = $1 ORDER BY code`},
	{"journal", `
	SELECT e.entry_no::text, e.entry_date::text, COALESCE(e.period_date::text, '') AS period_date, e.kind::text,
	       e.description, a.code AS account_code, a.name AS account_name, COALESCE(c.name, '') AS contact,
	       to_char(l.debit / 100.0, 'FM999999999999990.00') AS debit, to_char(l.credit / 100.0, 'FM999999999999990.00') AS credit
	FROM journal_entries e
	JOIN journal_lines l ON l.entry_id = e.id
	JOIN accounts a ON a.id = l.account_id
	LEFT JOIN contacts c ON c.id = l.contact_id
	WHERE e.user_id = $1
	ORDER BY e.entry_no, l.id`},
	{"contacts", `
	SELECT c.name, COALESCE(u.name, '') AS linked_member, c.created_at::text
	FROM contacts c LEFT JOIN users u ON u.id = c.linked_user_id
	WHERE c.user_id = $1 ORDER BY lower(c.name)`},
	{"budgets", `
	SELECT b.period_start::text, a.code AS account_code, a.name AS account_name,
	       to_char(b.amount / 100.0, 'FM999999999999990.00') AS amount
	FROM budgets b JOIN accounts a ON a.id = b.account_id
	WHERE b.user_id = $1 ORDER BY b.period_start, a.code`},
}

func (r *LedgerRepository) ExportTables(ctx context.Context, userID uuid.UUID) ([]model.ExportTable, error) {
	out := make([]model.ExportTable, 0, len(exports))
	for _, e := range exports {
		t, err := queryTable(ctx, r.db, e.name, e.query, userID)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func queryTable(ctx context.Context, db *sql.DB, name, query string, userID uuid.UUID) (model.ExportTable, error) {
	t := model.ExportTable{Name: name, Rows: [][]string{}}
	rows, err := db.QueryContext(ctx, query, userID)
	if err != nil {
		return t, err
	}
	defer rows.Close()
	if t.Columns, err = rows.Columns(); err != nil {
		return t, err
	}
	for rows.Next() {
		row := make([]string, len(t.Columns))
		ptrs := make([]any, len(row))
		for i := range row {
			ptrs[i] = &row[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return t, err
		}
		t.Rows = append(t.Rows, row)
	}
	return t, rows.Err()
}
