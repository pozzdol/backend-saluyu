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
	ErrAccountNotFound   = errors.New("account not found")
	ErrAccountCodeTaken  = errors.New("account code already used")
	ErrEntryNotFound     = errors.New("entry not found")
	ErrDuplicateClient   = errors.New("an entry with this client id exists")
	ErrOpeningLocked     = errors.New("the opening entry can only be changed on the opening balance page")
	ErrBooksNotEmpty     = errors.New("opening balance can only change before other transactions exist")
	ErrStartAfterEntries = errors.New("the start date is after your first transaction; pick that date or earlier")
)

const accountColumns = `id, code, name, type, category, normal_balance, is_active, COALESCE(icon, ''), COALESCE(color, ''), kind`

type LedgerRepository struct {
	db *sql.DB
}

func NewLedgerRepository(db *sql.DB) *LedgerRepository {
	return &LedgerRepository{db: db}
}

// seedAccounts copies the default chart of accounts to a new user inside the register transaction.
func seedAccounts(ctx context.Context, tx *sql.Tx, userID uuid.UUID) error {
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO accounts (user_id, code, name, type, category, normal_balance, icon, color) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, a := range model.DefaultAccounts {
		if _, err := stmt.ExecContext(ctx, userID, a.Code, a.Name, a.Type, a.Category, model.NormalBalance(a.Type), a.Icon, a.Color); err != nil {
			return err
		}
	}
	return nil
}

func scanAccounts(rows *sql.Rows) ([]model.Account, error) {
	defer rows.Close()
	var out []model.Account
	for rows.Next() {
		var a model.Account
		if err := rows.Scan(&a.ID, &a.Code, &a.Name, &a.Type, &a.Category, &a.NormalBalance, &a.IsActive, &a.Icon, &a.Color, &a.Kind); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListAccounts also counts each account's journal lines of the last 30 days, so pickers can
// put the categories a person actually uses first.
func (r *LedgerRepository) ListAccounts(ctx context.Context, userID uuid.UUID) ([]model.Account, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT `+accountColumns+`,
	       (SELECT count(*) FROM journal_lines l JOIN journal_entries e ON e.id = l.entry_id
	        WHERE l.account_id = a.id AND e.entry_date >= CURRENT_DATE - 30)
	FROM accounts a WHERE user_id = $1 ORDER BY code`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Account{}
	for rows.Next() {
		var a model.Account
		if err := rows.Scan(&a.ID, &a.Code, &a.Name, &a.Type, &a.Category, &a.NormalBalance, &a.IsActive, &a.Icon, &a.Color, &a.Kind, &a.Uses30d); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// FindAccounts returns the requested accounts that belong to userID, keyed by ID.
// Accounts of other users are simply missing from the map.
func (r *LedgerRepository) FindAccounts(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]model.Account, error) {
	strIDs := make([]string, len(ids))
	for i, id := range ids {
		strIDs[i] = id.String()
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE user_id = $1 AND id = ANY($2::uuid[])`, userID, strIDs)
	if err != nil {
		return nil, err
	}
	list, err := scanAccounts(rows)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]model.Account, len(list))
	for _, a := range list {
		out[a.ID] = a
	}
	return out, nil
}

func (r *LedgerRepository) CreateAccount(ctx context.Context, userID uuid.UUID, a *model.Account) error {
	err := r.db.QueryRowContext(ctx, `
	INSERT INTO accounts (user_id, code, name, type, category, normal_balance, icon, color, kind)
	VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), COALESCE(NULLIF($9, ''), 'general'))
	RETURNING id, is_active, kind`,
		userID, a.Code, a.Name, a.Type, a.Category, a.NormalBalance, a.Icon, a.Color, a.Kind).Scan(&a.ID, &a.IsActive, &a.Kind)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrAccountCodeTaken
	}
	return err
}

func (r *LedgerRepository) UpdateAccount(ctx context.Context, userID, id uuid.UUID, req model.UpdateAccountRequest) (*model.Account, error) {
	var a model.Account
	err := r.db.QueryRowContext(ctx, `
	UPDATE accounts
	SET name = COALESCE($3, name), category = COALESCE($4, category),
	    is_active = COALESCE($5, is_active), icon = COALESCE($6, icon), color = COALESCE($7, color),
	    updated_at = now()
	WHERE id = $1 AND user_id = $2
	RETURNING `+accountColumns, id, userID, req.Name, req.Category, req.IsActive, req.Icon, req.Color).
		Scan(&a.ID, &a.Code, &a.Name, &a.Type, &a.Category, &a.NormalBalance, &a.IsActive, &a.Icon, &a.Color, &a.Kind)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// CreateEntry stores an entry and its lines atomically and assigns the next per-user entry_no.
func (r *LedgerRepository) CreateEntry(ctx context.Context, userID uuid.UUID, e *model.Entry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after Commit

	if err := lockUser(ctx, tx, userID); err != nil {
		return err
	}
	if err := insertEntry(ctx, tx, userID, e); err != nil {
		return err
	}
	if err := createLoanRequest(ctx, tx, e); err != nil {
		return err
	}
	return tx.Commit()
}

// lockUser serializes writes of one user's books: two requests cannot both take
// MAX(entry_no)+1, and the opening check cannot race a new entry.
func lockUser(ctx context.Context, tx *sql.Tx, userID uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, userID)
	return err
}

func insertEntry(ctx context.Context, tx *sql.Tx, userID uuid.UUID, e *model.Entry) error {
	err := tx.QueryRowContext(ctx, `
	INSERT INTO journal_entries (user_id, entry_no, entry_date, kind, description, period_date, client_id)
	VALUES ($1, (SELECT COALESCE(MAX(entry_no), 0) + 1 FROM journal_entries WHERE user_id = $1), $2, $3, $4, $5, $6)
	RETURNING id, entry_no, created_at`,
		userID, e.EntryDate, e.Kind, e.Description, e.PeriodDate, e.ClientID).Scan(&e.ID, &e.EntryNo, &e.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == "journal_entries_client_idx" {
		return ErrDuplicateClient
	}
	if err != nil {
		return err
	}
	return insertLines(ctx, tx, e)
}

func insertLines(ctx context.Context, tx *sql.Tx, e *model.Entry) error {
	for _, l := range e.Lines {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO journal_lines (entry_id, account_id, debit, credit, contact_id) VALUES ($1, $2, $3, $4, $5)`,
			e.ID, l.AccountID, l.Debit, l.Credit, l.ContactID); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceOpening sets the opening entry and the start date. before is the current opening
// entry (nil if none) and e the new one (nil when every balance is zero). An existing opening
// is edited in place, so it keeps its history. The books may already hold other entries, as
// long as none is dated before the new start date.
func (r *LedgerRepository) ReplaceOpening(ctx context.Context, userID uuid.UUID, startDate time.Time, before, e *model.Entry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := lockUser(ctx, tx, userID); err != nil {
		return err
	}
	var earlier bool
	err = tx.QueryRowContext(ctx, `
	SELECT EXISTS (SELECT 1 FROM journal_entries WHERE user_id = $1 AND kind <> 'opening' AND entry_date < $2)`,
		userID, startDate).Scan(&earlier)
	if err != nil {
		return err
	}
	if earlier {
		return ErrStartAfterEntries
	}

	switch {
	case before != nil && e != nil:
		e.ID, e.EntryNo, e.CreatedAt = before.ID, before.EntryNo, before.CreatedAt
		err = replaceInTx(ctx, tx, userID, before, e)
	case before != nil:
		_, err = tx.ExecContext(ctx, `DELETE FROM journal_entries WHERE id = $1`, before.ID)
	case e != nil:
		err = insertEntry(ctx, tx, userID, e)
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET start_date = $2, updated_at = now() WHERE id = $1`, userID, startDate); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *LedgerRepository) StartDate(ctx context.Context, userID uuid.UUID) (*time.Time, error) {
	var d *time.Time
	err := r.db.QueryRowContext(ctx, `SELECT start_date FROM users WHERE id = $1`, userID).Scan(&d)
	return d, err
}

// EntryByClientID finds the entry a client already sent with this id.
func (r *LedgerRepository) EntryByClientID(ctx context.Context, userID, clientID uuid.UUID) (*model.Entry, error) {
	var id uuid.UUID
	err := r.db.QueryRowContext(ctx, `SELECT id FROM journal_entries WHERE user_id = $1 AND client_id = $2`, userID, clientID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrEntryNotFound
	}
	if err != nil {
		return nil, err
	}
	return r.GetEntry(ctx, userID, id)
}

// LockedUntil is the last day of the closed books, or nil when nothing is closed.
func (r *LedgerRepository) LockedUntil(ctx context.Context, userID uuid.UUID) (*time.Time, error) {
	var d *time.Time
	err := r.db.QueryRowContext(ctx, `SELECT locked_until FROM users WHERE id = $1`, userID).Scan(&d)
	return d, err
}

func (r *LedgerRepository) PeriodStartDay(ctx context.Context, userID uuid.UUID) (int, error) {
	var d int
	err := r.db.QueryRowContext(ctx, `SELECT period_start_day FROM users WHERE id = $1`, userID).Scan(&d)
	return d, err
}

// BalanceAsOf is an account's debits minus credits up to and including date.
func (r *LedgerRepository) BalanceAsOf(ctx context.Context, userID, accountID uuid.UUID, date time.Time) (int64, error) {
	var b int64
	err := r.db.QueryRowContext(ctx, `
	SELECT COALESCE(SUM(l.debit - l.credit), 0)
	FROM journal_lines l JOIN journal_entries e ON e.id = l.entry_id
	WHERE e.user_id = $1 AND l.account_id = $2 AND e.entry_date <= $3`, userID, accountID, date).Scan(&b)
	return b, err
}

func (r *LedgerRepository) DeleteEntry(ctx context.Context, userID, id uuid.UUID) error {
	var kind string
	err := r.db.QueryRowContext(ctx, `SELECT kind FROM journal_entries WHERE id = $1 AND user_id = $2`, id, userID).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrEntryNotFound
	}
	if err != nil {
		return err
	}
	if kind == "opening" {
		return ErrOpeningLocked
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// A request the other member has not approved goes with the entry. An approved one
	// (from_entry_id or to_entry_id) is RESTRICT: both books hold the loan, so neither side
	// may drop it alone.
	if _, err := tx.ExecContext(ctx, `DELETE FROM loan_requests WHERE from_entry_id = $1 AND status <> 'approved'`, id); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM journal_entries WHERE id = $1 AND user_id = $2`, id, userID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return ErrEntryLocked
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ListEntries returns one page of entries (newest first) with their lines, plus the total match count.
// Every filter is optional: a NULL parameter switches its condition off.
func (r *LedgerRepository) ListEntries(ctx context.Context, userID uuid.UUID, f model.EntryFilter) ([]model.Entry, int, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT e.id, e.entry_no, e.entry_date, e.period_date, e.kind, e.description, e.created_at,
	       (SELECT json_agg(json_build_object(
	                   'account_id', l.account_id, 'code', a.code, 'name', a.name,
	                   'debit', l.debit, 'credit', l.credit,
	                   'contact_id', l.contact_id, 'contact_name', c.name) ORDER BY l.id)
	        FROM journal_lines l JOIN accounts a ON a.id = l.account_id
	        LEFT JOIN contacts c ON c.id = l.contact_id
	        WHERE l.entry_id = e.id),
	       COUNT(*) OVER ()
	FROM journal_entries e
	WHERE e.user_id = $1
	  AND ($2::date IS NULL OR e.entry_date >= $2)
	  AND ($3::date IS NULL OR e.entry_date <= $3)
	  AND ($4::entry_kind IS NULL OR e.kind = $4)
	  AND ($5::uuid IS NULL OR EXISTS (
	        SELECT 1 FROM journal_lines l WHERE l.entry_id = e.id AND l.account_id = $5))
	  AND ($6::text = '' OR e.description ILIKE '%' || $6 || '%')
	  -- An entry's amount is its debit total (equal to its credit total).
	  AND (($9::bigint IS NULL AND $10::bigint IS NULL) OR
	       (SELECT SUM(debit) FROM journal_lines WHERE entry_id = e.id) BETWEEN COALESCE($9, 0) AND COALESCE($10, 9223372036854775807))
	ORDER BY e.entry_date DESC, e.entry_no DESC
	LIMIT $7 OFFSET $8`,
		userID, f.From, f.To, f.Kind, f.AccountID, f.Query, f.Limit, f.Offset, f.AmountMin, f.AmountMax)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []model.Entry{}
	total := 0
	for rows.Next() {
		var e model.Entry
		var lines []byte
		if err := rows.Scan(&e.ID, &e.EntryNo, &e.EntryDate, &e.PeriodDate, &e.Kind, &e.Description, &e.CreatedAt, &lines, &total); err != nil {
			return nil, 0, err
		}
		if err := json.Unmarshal(lines, &e.Lines); err != nil {
			return nil, 0, err
		}
		e.Date = e.EntryDate.Format(time.DateOnly)
		e.PeriodDateStr = model.DateString(e.PeriodDate)
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// LedgerLines returns the raw debit/credit lines of one account in date order, plus the
// debit-minus-credit sum of everything before `from` (the opening position).
func (r *LedgerRepository) LedgerLines(ctx context.Context, userID, accountID uuid.UUID, from, to *time.Time) (int64, []model.LedgerRow, error) {
	var opening int64
	if from != nil {
		err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(l.debit - l.credit), 0)
		FROM journal_lines l JOIN journal_entries e ON e.id = l.entry_id
		WHERE e.user_id = $1 AND l.account_id = $2 AND e.entry_date < $3`, userID, accountID, from).Scan(&opening)
		if err != nil {
			return 0, nil, err
		}
	}

	rows, err := r.db.QueryContext(ctx, `
	SELECT e.id, e.entry_no, e.entry_date, e.kind, e.description, l.debit, l.credit,
	       COALESCE((SELECT string_agg(a.name, ', ' ORDER BY a.code)
	                 FROM journal_lines o JOIN accounts a ON a.id = o.account_id
	                 WHERE o.entry_id = e.id AND o.id <> l.id), '')
	FROM journal_lines l JOIN journal_entries e ON e.id = l.entry_id
	WHERE e.user_id = $1 AND l.account_id = $2
	  AND ($3::date IS NULL OR e.entry_date >= $3)
	  AND ($4::date IS NULL OR e.entry_date <= $4)
	ORDER BY e.entry_date, e.entry_no, l.id`, userID, accountID, from, to)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	out := []model.LedgerRow{}
	for rows.Next() {
		var row model.LedgerRow
		var date time.Time
		if err := rows.Scan(&row.EntryID, &row.EntryNo, &date, &row.Kind, &row.Description, &row.Debit, &row.Credit, &row.Counter); err != nil {
			return 0, nil, err
		}
		row.Date = date.Format(time.DateOnly)
		out = append(out, row)
	}
	return opening, out, rows.Err()
}

// TrialBalanceTotals sums every account's debits and credits up to and including asOf.
// Accounts without lines are included with zeros.
func (r *LedgerRepository) TrialBalanceTotals(ctx context.Context, userID uuid.UUID, asOf time.Time) ([]model.TrialBalanceRow, error) {
	rows, err := r.db.QueryContext(ctx, `
	SELECT a.id, a.code, a.name, a.type, COALESCE(SUM(l.debit), 0), COALESCE(SUM(l.credit), 0)
	FROM accounts a
	LEFT JOIN (journal_lines l JOIN journal_entries e ON e.id = l.entry_id AND e.entry_date <= $2)
	       ON l.account_id = a.id
	WHERE a.user_id = $1
	GROUP BY a.id
	ORDER BY a.code`, userID, asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.TrialBalanceRow{}
	for rows.Next() {
		var row model.TrialBalanceRow
		if err := rows.Scan(&row.AccountID, &row.Code, &row.Name, &row.Type, &row.Debit, &row.Credit); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
