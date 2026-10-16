package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/period"
	"github.com/pozzdol/backend-saluyu/internal/repository"
)

var (
	ErrAccountNotFound  = repository.ErrAccountNotFound
	ErrAccountCodeTaken = repository.ErrAccountCodeTaken
	ErrEntryNotFound    = repository.ErrEntryNotFound
	ErrAccountInactive  = errors.New("account is inactive")
	ErrSameAccount      = errors.New("from and to account must differ")
	ErrWrongAccountType = errors.New("account type does not fit this kind of transaction")
	ErrUnbalanced       = errors.New("journal entry is not balanced")
	ErrOpeningLocked    = repository.ErrOpeningLocked
	ErrBooksNotEmpty    = repository.ErrBooksNotEmpty
	ErrNotOpened        = errors.New("set your opening balance before recording transactions")
	ErrBeforeStart      = errors.New("date is before the day your books open")
)

// LedgerRepository is what the ledger needs from storage. *repository.LedgerRepository implements it.
type LedgerRepository interface {
	ListAccounts(ctx context.Context, userID uuid.UUID) ([]model.Account, error)
	FindAccounts(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]model.Account, error)
	CreateAccount(ctx context.Context, userID uuid.UUID, a *model.Account) error
	UpdateAccount(ctx context.Context, userID, id uuid.UUID, req model.UpdateAccountRequest) (*model.Account, error)
	CreateEntry(ctx context.Context, userID uuid.UUID, e *model.Entry) error
	DeleteEntry(ctx context.Context, userID, id uuid.UUID) error
	ListEntries(ctx context.Context, userID uuid.UUID, f model.EntryFilter) ([]model.Entry, int, error)
	LedgerLines(ctx context.Context, userID, accountID uuid.UUID, from, to *time.Time) (int64, []model.LedgerRow, error)
	TrialBalanceTotals(ctx context.Context, userID uuid.UUID, asOf time.Time) ([]model.TrialBalanceRow, error)
	StartDate(ctx context.Context, userID uuid.UUID) (*time.Time, error)
	PeriodStartDay(ctx context.Context, userID uuid.UUID) (int, error)
	BalanceAsOf(ctx context.Context, userID, accountID uuid.UUID, date time.Time) (int64, error)
	ReplaceOpening(ctx context.Context, userID uuid.UUID, startDate time.Time, e *model.Entry) error
	ListContacts(ctx context.Context, userID uuid.UUID, only *uuid.UUID) ([]model.Contact, error)
	ContactIDs(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]bool, error)
	CreateContact(ctx context.Context, userID uuid.UUID, name string) (*model.Contact, error)
	RenameContact(ctx context.Context, userID, id uuid.UUID, name string) error
	DeleteContact(ctx context.Context, userID, id uuid.UUID) error
	ContactHistory(ctx context.Context, userID, contactID uuid.UUID) ([]model.ContactLine, error)
}

type LedgerService struct {
	repo LedgerRepository
}

func NewLedgerService(repo LedgerRepository) *LedgerService {
	return &LedgerService{repo: repo}
}

func (s *LedgerService) ListAccounts(ctx context.Context, userID uuid.UUID) ([]model.Account, error) {
	return s.repo.ListAccounts(ctx, userID)
}

func (s *LedgerService) CreateAccount(ctx context.Context, userID uuid.UUID, req model.CreateAccountRequest) (*model.Account, error) {
	a := &model.Account{
		Code: req.Code, Name: req.Name, Type: req.Type, Category: req.Category,
		NormalBalance: model.NormalBalance(req.Type),
	}
	if err := s.repo.CreateAccount(ctx, userID, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *LedgerService) UpdateAccount(ctx context.Context, userID, id uuid.UUID, req model.UpdateAccountRequest) (*model.Account, error) {
	return s.repo.UpdateAccount(ctx, userID, id, req)
}

// kindRules says which account types a simple form may move money between: [from, to].
var kindRules = map[string][2]string{
	"income":   {"Revenue", "Asset"}, // Dr Bank / Cr Gaji
	"expense":  {"Asset", "Expense"}, // Dr Makan / Cr Bank
	"transfer": {"Asset", "Asset"},   // Dr Tabungan / Cr Bank
}

// PostEntry turns a simple form into a two-line journal entry: money leaves `from` (credit)
// and arrives in `to` (debit). Refunds are income to 4003 Pengembalian, so no form ever
// credits an expense account and expense balances cannot go negative.
func (s *LedgerService) PostEntry(ctx context.Context, userID uuid.UUID, req model.PostEntryRequest) (*model.Entry, error) {
	if req.FromAccountID == req.ToAccountID {
		return nil, ErrSameAccount
	}
	date, err := time.Parse(time.DateOnly, req.EntryDate) // format already checked by binding
	if err != nil {
		return nil, err
	}

	accounts, err := s.repo.FindAccounts(ctx, userID, []uuid.UUID{req.FromAccountID, req.ToAccountID})
	if err != nil {
		return nil, err
	}
	from, okFrom := accounts[req.FromAccountID]
	to, okTo := accounts[req.ToAccountID]
	if !okFrom || !okTo {
		return nil, ErrAccountNotFound
	}
	rule := kindRules[req.Kind]
	if from.Type != rule[0] || to.Type != rule[1] {
		return nil, fmt.Errorf("%w: %s needs %s -> %s", ErrWrongAccountType, req.Kind, rule[0], rule[1])
	}

	e := &model.Entry{
		EntryDate:   date,
		Kind:        req.Kind,
		Description: req.Description,
		Lines: []model.Line{
			{AccountID: to.ID, Debit: req.Amount},
			{AccountID: from.ID, Credit: req.Amount},
		},
	}
	// ponytail: next_period is ignored for other kinds; only early salary needs it.
	if req.NextPeriod && req.Kind == "income" {
		day, err := s.repo.PeriodStartDay(ctx, userID)
		if err != nil {
			return nil, err
		}
		next := period.For(date, day).Next
		e.PeriodDate = &next
	}
	if err := s.post(ctx, userID, e, accounts); err != nil {
		return nil, err
	}
	return e, nil
}

// post is the single way a journal entry gets written. Later phases (cash count,
// contacts, shared loans) build their own lines and call it too.
// accounts must contain every account used by the lines, already filtered to this user.
func (s *LedgerService) post(ctx context.Context, userID uuid.UUID, e *model.Entry, accounts map[uuid.UUID]model.Account) error {
	// ponytail: start date read outside the insert transaction; the opening can only change
	// while the books are empty, so the window is a user posting in two tabs during onboarding.
	start, err := s.repo.StartDate(ctx, userID)
	if err != nil {
		return err
	}
	if start == nil {
		return ErrNotOpened
	}
	if e.EntryDate.Before(*start) {
		return fmt.Errorf("%w (%s)", ErrBeforeStart, start.Format(time.DateOnly))
	}
	if err := validate(e, accounts); err != nil {
		return err
	}
	if err := s.checkContacts(ctx, userID, e); err != nil {
		return err
	}
	return s.repo.CreateEntry(ctx, userID, e)
}

// checkContacts makes sure every contact on the lines belongs to this user.
func (s *LedgerService) checkContacts(ctx context.Context, userID uuid.UUID, e *model.Entry) error {
	var ids []uuid.UUID
	for _, l := range e.Lines {
		if l.ContactID != nil {
			ids = append(ids, *l.ContactID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	owned, err := s.repo.ContactIDs(ctx, userID, ids)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if !owned[id] {
			return ErrContactNotFound
		}
	}
	return nil
}

// validate checks that every line uses an active account of this user, has exactly one
// positive side, and that debits equal credits. It also fills in account code and name.
func validate(e *model.Entry, accounts map[uuid.UUID]model.Account) error {
	var debit, credit int64
	for i, l := range e.Lines {
		a, ok := accounts[l.AccountID]
		if !ok {
			return ErrAccountNotFound
		}
		if !a.IsActive {
			return fmt.Errorf("%w: %s %s", ErrAccountInactive, a.Code, a.Name)
		}
		if (l.Debit == 0) == (l.Credit == 0) || l.Debit < 0 || l.Credit < 0 {
			return ErrUnbalanced // a line must have exactly one positive side
		}
		// Receivables and payables are per person; nothing else carries a person.
		if perContact(a.Code) != (l.ContactID != nil) {
			if l.ContactID == nil {
				return ErrNeedsContact
			}
			return ErrContactNotAllowed
		}
		e.Lines[i].Code, e.Lines[i].Name = a.Code, a.Name
		debit += l.Debit
		credit += l.Credit
	}
	if debit != credit || len(e.Lines) < 2 {
		return ErrUnbalanced
	}
	e.Date = e.EntryDate.Format(time.DateOnly)
	e.PeriodDateStr = model.DateString(e.PeriodDate)
	return nil
}

func (s *LedgerService) ListEntries(ctx context.Context, userID uuid.UUID, f model.EntryFilter) ([]model.Entry, int, error) {
	return s.repo.ListEntries(ctx, userID, f)
}

func (s *LedgerService) DeleteEntry(ctx context.Context, userID, id uuid.UUID) error {
	return s.repo.DeleteEntry(ctx, userID, id)
}

// signed turns a debit-minus-credit amount into a balance that is positive on the
// account's normal side: Rp 100 in Bank is +100, Rp 100 of Gaji earned is also +100.
func signed(a model.Account, debitMinusCredit int64) int64 {
	if a.NormalBalance == "Debit" {
		return debitMinusCredit
	}
	return -debitMinusCredit
}

// Ledger is one account's history with a running balance, like the Excel "Buku Besar" sheet.
func (s *LedgerService) Ledger(ctx context.Context, userID, accountID uuid.UUID, from, to *time.Time) (*model.Ledger, error) {
	accounts, err := s.repo.FindAccounts(ctx, userID, []uuid.UUID{accountID})
	if err != nil {
		return nil, err
	}
	account, ok := accounts[accountID]
	if !ok {
		return nil, ErrAccountNotFound
	}

	opening, rows, err := s.repo.LedgerLines(ctx, userID, accountID, from, to)
	if err != nil {
		return nil, err
	}

	balance := signed(account, opening)
	l := &model.Ledger{Account: account, Opening: balance, Rows: rows}
	for i := range rows {
		balance += signed(account, rows[i].Debit-rows[i].Credit)
		rows[i].Balance = balance
	}
	l.Closing = balance
	return l, nil
}

// TrialBalance puts every account's net balance on its debit or credit side.
// Because every entry is balanced, the two totals must be equal.
func (s *LedgerService) TrialBalance(ctx context.Context, userID uuid.UUID, asOf time.Time) (*model.TrialBalance, error) {
	rows, err := s.repo.TrialBalanceTotals(ctx, userID, asOf)
	if err != nil {
		return nil, err
	}

	tb := &model.TrialBalance{AsOf: asOf.Format(time.DateOnly), Rows: rows}
	for i := range rows {
		if net := rows[i].Debit - rows[i].Credit; net >= 0 {
			rows[i].DebitBalance = net
		} else {
			rows[i].CreditBalance = -net
		}
		tb.TotalDebit += rows[i].DebitBalance
		tb.TotalCredit += rows[i].CreditBalance
	}
	tb.Balanced = tb.TotalDebit == tb.TotalCredit
	return tb, nil
}
