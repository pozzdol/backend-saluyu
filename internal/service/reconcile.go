package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
)

var (
	ErrNotBalanceSheet  = errors.New("only asset and liability accounts have an opening or statement balance")
	ErrNeedsContact     = errors.New("receivables and payables need a contact")
	ErrDuplicateAccount = errors.New("an account appears twice")
	ErrWrongCounter     = errors.New("the difference must go to a revenue or expense account")
)

// Codes from the default chart of accounts that the ledger itself relies on.
const (
	codeOpeningEquity = "3001" // Ekuitas Saldo Awal
	codeOtherIncome   = "4199" // Pendapatan Lain-lain: cash found
	codeCashExpense   = "5199" // Beban Tunai Lain-lain: cash spent but not recorded
	codeReceivable    = "1101"
	codePayable       = "2101"
)

// openingLines turns real balances into one balanced opening entry: assets on the debit side,
// liabilities on the credit side, and the difference (net worth) on 3001 Ekuitas Saldo Awal.
// Zero balances are skipped. All zero returns no lines.
func openingLines(items []model.OpeningItem, accounts map[uuid.UUID]model.Account, equityID uuid.UUID) []model.Line {
	var lines []model.Line
	var net int64 // assets - liabilities
	for _, it := range items {
		if it.Amount == 0 {
			continue
		}
		if accounts[it.AccountID].Type == "Asset" {
			lines = append(lines, model.Line{AccountID: it.AccountID, ContactID: it.ContactID, Debit: it.Amount})
			net += it.Amount
		} else {
			lines = append(lines, model.Line{AccountID: it.AccountID, ContactID: it.ContactID, Credit: it.Amount})
			net -= it.Amount
		}
	}
	switch {
	case net > 0:
		lines = append(lines, model.Line{AccountID: equityID, Credit: net})
	case net < 0: // more debt than assets
		lines = append(lines, model.Line{AccountID: equityID, Debit: -net})
	}
	return lines
}

// adjustLines moves acc's balance by diff (on its normal side) against counterID.
// diff > 0 grows the balance: debit an asset, credit a liability. diff 0 returns nil.
func adjustLines(acc model.Account, counterID uuid.UUID, diff int64) []model.Line {
	if diff == 0 {
		return nil
	}
	amount := diff
	if amount < 0 {
		amount = -amount
	}
	accLine := model.Line{AccountID: acc.ID}
	counter := model.Line{AccountID: counterID}
	if (diff > 0) == (acc.NormalBalance == "Debit") {
		accLine.Debit, counter.Credit = amount, amount
	} else {
		accLine.Credit, counter.Debit = amount, amount
	}
	return []model.Line{accLine, counter}
}

// byCode indexes the user's accounts by code.
func (s *LedgerService) byCode(ctx context.Context, userID uuid.UUID) (map[string]model.Account, error) {
	list, err := s.repo.ListAccounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]model.Account, len(list))
	for _, a := range list {
		out[a.Code] = a
	}
	return out, nil
}

func (s *LedgerService) GetOpening(ctx context.Context, userID uuid.UUID) (*model.Opening, error) {
	start, err := s.repo.StartDate(ctx, userID)
	if err != nil {
		return nil, err
	}
	o := &model.Opening{StartDate: model.DateString(start)}

	kind := "opening"
	entries, _, err := s.repo.ListEntries(ctx, userID, model.EntryFilter{Kind: &kind, Limit: 1})
	if err != nil {
		return nil, err
	}
	if len(entries) == 1 {
		o.Entry = &entries[0]
	}
	// Always editable now: edits keep a revision, and the start date cannot pass the first entry.
	o.Editable = true
	return o, nil
}

// SetOpening opens (or re-opens) the books on start_date with real balances.
// It works only while the books hold no other transactions.
func (s *LedgerService) SetOpening(ctx context.Context, userID uuid.UUID, req model.OpeningRequest) (*model.Opening, error) {
	start, err := time.Parse(time.DateOnly, req.StartDate)
	if err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, 0, len(req.Items))
	type key struct{ account, contact uuid.UUID }
	seen := map[key]bool{}
	for _, it := range req.Items {
		k := key{account: it.AccountID}
		if it.ContactID != nil {
			k.contact = *it.ContactID
		}
		if seen[k] {
			return nil, ErrDuplicateAccount
		}
		seen[k] = true
		ids = append(ids, it.AccountID)
	}
	codes, err := s.byCode(ctx, userID)
	if err != nil {
		return nil, err
	}
	equity := codes[codeOpeningEquity]
	accounts, err := s.repo.FindAccounts(ctx, userID, append(ids, equity.ID))
	if err != nil {
		return nil, err
	}
	for _, it := range req.Items {
		a, ok := accounts[it.AccountID]
		switch {
		case !ok:
			return nil, ErrAccountNotFound
		case a.Type != "Asset" && a.Type != "Liability":
			return nil, fmt.Errorf("%w: %s %s", ErrNotBalanceSheet, a.Code, a.Name)
		case perContact(a.Code) && it.ContactID == nil && it.Amount > 0:
			return nil, ErrNeedsContact
		case !perContact(a.Code) && it.ContactID != nil:
			return nil, ErrContactNotAllowed
		}
	}
	if _, ok := accounts[equity.ID]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrAccountNotFound, codeOpeningEquity)
	}

	var e *model.Entry
	if lines := openingLines(req.Items, accounts, equity.ID); len(lines) > 0 {
		e = &model.Entry{EntryDate: start, Kind: "opening", Description: "Saldo awal", Lines: lines}
		if err := validate(e, accounts); err != nil {
			return nil, err
		}
		if err := s.checkContacts(ctx, userID, e); err != nil {
			return nil, err
		}
	}
	current, err := s.GetOpening(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.ReplaceOpening(ctx, userID, start, current.Entry, e); err != nil {
		return nil, err
	}
	return s.GetOpening(ctx, userID)
}

// Balance is an account's balance on its normal side at the end of date.
func (s *LedgerService) Balance(ctx context.Context, userID, accountID uuid.UUID, date time.Time) (int64, error) {
	accounts, err := s.repo.FindAccounts(ctx, userID, []uuid.UUID{accountID})
	if err != nil {
		return 0, err
	}
	a, ok := accounts[accountID]
	if !ok {
		return 0, ErrAccountNotFound
	}
	raw, err := s.repo.BalanceAsOf(ctx, userID, accountID, date)
	if err != nil {
		return 0, err
	}
	return signed(a, raw), nil
}

// CashCount posts the gap between counted cash and the books: missing cash is spending that
// was never recorded (5199), extra cash is income (4199). Expense balances never go negative.
func (s *LedgerService) CashCount(ctx context.Context, userID uuid.UUID, req model.ReconcileRequest) (*model.ReconcileResult, error) {
	codes, err := s.byCode(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.reconcile(ctx, userID, req, "cash_count", func(diff int64) uuid.UUID {
		if diff < 0 {
			return codes[codeCashExpense].ID
		}
		return codes[codeOtherIncome].ID
	})
}

// Reconcile matches an account to a bank or e-wallet statement, posting the gap to the
// revenue or expense account the user picks (bank interest, admin fee, ...).
func (s *LedgerService) Reconcile(ctx context.Context, userID uuid.UUID, req model.ReconcileRequest) (*model.ReconcileResult, error) {
	if req.CounterAccountID == nil {
		return nil, ErrWrongCounter
	}
	return s.reconcile(ctx, userID, req, "adjustment", func(int64) uuid.UUID { return *req.CounterAccountID })
}

func (s *LedgerService) reconcile(ctx context.Context, userID uuid.UUID, req model.ReconcileRequest, kind string, counterFor func(diff int64) uuid.UUID) (*model.ReconcileResult, error) {
	date, err := time.Parse(time.DateOnly, req.Date)
	if err != nil {
		return nil, err
	}
	book, err := s.Balance(ctx, userID, req.AccountID, date)
	if err != nil {
		return nil, err
	}
	res := &model.ReconcileResult{Book: book, Actual: req.Actual, Diff: req.Actual - book}
	if res.Diff == 0 {
		return res, nil
	}

	counterID := counterFor(res.Diff)
	accounts, err := s.repo.FindAccounts(ctx, userID, []uuid.UUID{req.AccountID, counterID})
	if err != nil {
		return nil, err
	}
	acc, counter := accounts[req.AccountID], accounts[counterID]
	switch {
	case acc.Type != "Asset" && acc.Type != "Liability":
		return nil, fmt.Errorf("%w: %s %s", ErrNotBalanceSheet, acc.Code, acc.Name)
	case acc.Code == codeReceivable || acc.Code == codePayable:
		return nil, ErrNeedsContact
	case counter.ID == uuid.Nil:
		return nil, ErrAccountNotFound
	case counter.Type != "Revenue" && counter.Type != "Expense":
		return nil, ErrWrongCounter
	}

	desc := fmt.Sprintf("Hitung kas: fisik %s, buku %s", rupiah(req.Actual), rupiah(book))
	if kind == "adjustment" {
		desc = fmt.Sprintf("Rekonsiliasi: saldo rekening %s, buku %s", rupiah(req.Actual), rupiah(book))
	}
	e := &model.Entry{EntryDate: date, Kind: kind, Description: desc, Lines: adjustLines(acc, counterID, res.Diff)}
	if err := s.post(ctx, userID, e, accounts); err != nil {
		return nil, err
	}
	res.Entry = e
	return res, nil
}

// rupiah formats sen as "Rp1.234.567,89" for entry descriptions.
func rupiah(sen int64) string {
	neg := sen < 0
	if neg {
		sen = -sen
	}
	whole := fmt.Sprint(sen / 100)
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "." + whole[i:]
	}
	out := "Rp" + whole
	if cents := sen % 100; cents != 0 {
		out += fmt.Sprintf(",%02d", cents)
	}
	if neg {
		out = "-" + out
	}
	return out
}
