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
	ErrAccountNotFound   = repository.ErrAccountNotFound
	ErrAccountCodeTaken  = repository.ErrAccountCodeTaken
	ErrEntryNotFound     = repository.ErrEntryNotFound
	ErrAccountInactive   = errors.New("account is inactive")
	ErrSameAccount       = errors.New("from and to account must differ")
	ErrWrongAccountType  = errors.New("account type does not fit this kind of transaction")
	ErrWrongFee          = errors.New("a fee needs a transfer and an expense account")
	ErrUnbalanced        = errors.New("journal entry is not balanced")
	ErrOpeningLocked     = repository.ErrOpeningLocked
	ErrBooksNotEmpty     = repository.ErrBooksNotEmpty
	ErrStartAfterEntries = repository.ErrStartAfterEntries
	ErrNotOpened         = errors.New("set your opening balance before recording transactions")
	ErrBeforeStart       = errors.New("date is before the day your books open")
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
	ReplaceOpening(ctx context.Context, userID uuid.UUID, startDate time.Time, before, e *model.Entry) error
	GetEntry(ctx context.Context, userID, id uuid.UUID) (*model.Entry, error)
	LoanLink(ctx context.Context, entryID uuid.UUID) (status string, requestID uuid.UUID, sender bool, other uuid.UUID, err error)
	ReplaceEntry(ctx context.Context, userID uuid.UUID, before, after *model.Entry) error
	Revisions(ctx context.Context, entryID uuid.UUID) ([]model.Revision, error)
	ProposeChange(ctx context.Context, loanRequestID, userID uuid.UUID, p model.ChangeProposal) error
	ChangePending(ctx context.Context, loanRequestID uuid.UUID) (bool, error)
	ListContacts(ctx context.Context, userID uuid.UUID, only *uuid.UUID) ([]model.Contact, error)
	ContactIDs(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]bool, error)
	CreateContact(ctx context.Context, userID uuid.UUID, name string) (*model.Contact, error)
	RenameContact(ctx context.Context, userID, id uuid.UUID, name string) error
	DeleteContact(ctx context.Context, userID, id uuid.UUID) error
	ContactHistory(ctx context.Context, userID, contactID uuid.UUID) ([]model.ContactLine, error)
	SharesActiveGroup(ctx context.Context, a, b uuid.UUID) (bool, error)
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
		Code: req.Code, Name: req.Name, Type: req.Type, Category: req.Category, Icon: req.Icon, Color: req.Color,
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

// fits says whether an account may sit on one side of a simple form. Cards (credit card,
// paylater) pay for things like a bank account does: the purchase raises the card's debt.
func fits(kind, side string, a model.Account) bool {
	money := a.Type == "Asset" || (a.Type == "Liability" && a.IsCard())
	switch kind + "/" + side {
	case "income/from":
		return a.Type == "Revenue" // Dr Bank / Cr Gaji
	case "income/to":
		return a.Type == "Asset"
	case "expense/from":
		return money // Dr Makan / Cr Bank, or Cr Kartu Kredit
	case "expense/to":
		return a.Type == "Expense"
	case "transfer/from":
		return money // Bank -> Tabungan, Bank -> Kartu (paying the bill)
	case "transfer/to":
		return a.Type == "Asset" || a.Type == "Liability"
	}
	return false
}

var sideWords = map[string][2]string{
	"income":   {"Revenue", "Asset"},
	"expense":  {"Asset or card", "Expense"},
	"transfer": {"Asset or card", "Asset or Liability"},
}

// PostEntry turns a simple form into a two-line journal entry: money leaves `from` (credit)
// and arrives in `to` (debit). Refunds are income to 4003 Pengembalian, so no form ever
// credits an expense account and expense balances cannot go negative.
func (s *LedgerService) PostEntry(ctx context.Context, userID uuid.UUID, req model.PostEntryRequest) (*model.Entry, error) {
	e, accounts, err := s.buildSimple(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	if err := s.post(ctx, userID, e, accounts); err != nil {
		return nil, err
	}
	return e, nil
}

// buildSimple turns the simple form into an entry without saving it; create and edit share it.
func (s *LedgerService) buildSimple(ctx context.Context, userID uuid.UUID, req model.PostEntryRequest) (*model.Entry, map[uuid.UUID]model.Account, error) {
	if req.FromAccountID == req.ToAccountID {
		return nil, nil, ErrSameAccount
	}
	date, err := time.Parse(time.DateOnly, req.EntryDate) // format already checked by binding
	if err != nil {
		return nil, nil, err
	}

	accounts, err := s.repo.FindAccounts(ctx, userID, []uuid.UUID{req.FromAccountID, req.ToAccountID})
	if err != nil {
		return nil, nil, err
	}
	from, okFrom := accounts[req.FromAccountID]
	to, okTo := accounts[req.ToAccountID]
	if !okFrom || !okTo {
		return nil, nil, ErrAccountNotFound
	}
	if !fits(req.Kind, "from", from) || !fits(req.Kind, "to", to) {
		w := sideWords[req.Kind]
		return nil, nil, fmt.Errorf("%w: %s needs %s -> %s", ErrWrongAccountType, req.Kind, w[0], w[1])
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
	// A transfer fee is one more expense line, paid from the same source.
	if req.Fee > 0 {
		if req.Kind != "transfer" || req.FeeAccountID == nil {
			return nil, nil, ErrWrongFee
		}
		fees, err := s.repo.FindAccounts(ctx, userID, []uuid.UUID{*req.FeeAccountID})
		if err != nil {
			return nil, nil, err
		}
		fee, ok := fees[*req.FeeAccountID]
		if !ok || fee.Type != "Expense" {
			return nil, nil, ErrWrongFee
		}
		accounts[fee.ID] = fee
		e.Lines = []model.Line{
			{AccountID: to.ID, Debit: req.Amount},
			{AccountID: fee.ID, Debit: req.Fee},
			{AccountID: from.ID, Credit: req.Amount + req.Fee},
		}
	}
	// ponytail: next_period is ignored for other kinds; only early salary needs it.
	if req.NextPeriod && req.Kind == "income" {
		day, err := s.repo.PeriodStartDay(ctx, userID)
		if err != nil {
			return nil, nil, err
		}
		next := period.For(date, day).Next
		e.PeriodDate = &next
	}
	return e, accounts, nil
}

// post is the single way a journal entry gets written. Later phases (cash count,
// contacts, shared loans) build their own lines and call it too.
// accounts must contain every account used by the lines, already filtered to this user.
func (s *LedgerService) post(ctx context.Context, userID uuid.UUID, e *model.Entry, accounts map[uuid.UUID]model.Account) error {
	if err := s.prepare(ctx, userID, e, accounts); err != nil {
		return err
	}
	return s.repo.CreateEntry(ctx, userID, e)
}

// prepare runs every check post does, without saving: for callers that store the entry
// together with other rows in their own transaction.
func (s *LedgerService) prepare(ctx context.Context, userID uuid.UUID, e *model.Entry, accounts map[uuid.UUID]model.Account) error {
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
	return s.checkContacts(ctx, userID, e)
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
