package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/repository"
)

var (
	ErrContactNotFound   = repository.ErrContactNotFound
	ErrContactNameTaken  = repository.ErrContactNameTaken
	ErrContactHasHistory = repository.ErrContactHasHistory
	ErrContactHasBalance = errors.New("settle this contact's balance before deleting")
	ErrContactNotAllowed = errors.New("only receivables (1101) and payables (2101) carry a contact")
	ErrOverpayment       = errors.New("payment is more than the outstanding balance")
	ErrNeedsExpense      = errors.New("choose the expense account for your own share")
)

func perContact(code string) bool { return code == codeReceivable || code == codePayable }

func (s *LedgerService) ListContacts(ctx context.Context, userID uuid.UUID) ([]model.Contact, error) {
	return s.repo.ListContacts(ctx, userID, nil)
}

func (s *LedgerService) CreateContact(ctx context.Context, userID uuid.UUID, req model.ContactRequest) (*model.Contact, error) {
	return s.repo.CreateContact(ctx, userID, req.Name)
}

func (s *LedgerService) RenameContact(ctx context.Context, userID, id uuid.UUID, req model.ContactRequest) (*model.Contact, error) {
	if err := s.repo.RenameContact(ctx, userID, id, req.Name); err != nil {
		return nil, err
	}
	return s.contact(ctx, userID, id)
}

// DeleteContact removes a contact nobody owes anything to. One with past transactions
// stays (the database refuses), because the history would lose its name.
func (s *LedgerService) DeleteContact(ctx context.Context, userID, id uuid.UUID) error {
	c, err := s.contact(ctx, userID, id)
	if err != nil {
		return err
	}
	if c.Receivable != 0 || c.Payable != 0 {
		return ErrContactHasBalance
	}
	return s.repo.DeleteContact(ctx, userID, id)
}

func (s *LedgerService) contact(ctx context.Context, userID, id uuid.UUID) (*model.Contact, error) {
	list, err := s.repo.ListContacts(ctx, userID, &id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrContactNotFound
	}
	return &list[0], nil
}

// ContactDetail is a contact's position plus the history of what changed it.
func (s *LedgerService) ContactDetail(ctx context.Context, userID, id uuid.UUID) (*model.ContactDetail, error) {
	c, err := s.contact(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	history, err := s.repo.ContactHistory(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	// Net is receivable - payable. A debit raises a receivable or lowers a payable, so
	// on either account it moves net by debit - credit.
	var net int64
	for i := range history {
		net += history[i].Debit - history[i].Credit
		history[i].RunningNet = net
	}
	return &model.ContactDetail{Contact: *c, History: history}, nil
}

// loanLines builds the journal lines of a loan form. receivable and payable are the
// contact's balances before this entry; they only matter for repayments.
func loanLines(req model.LoanRequest, receivableID, payableID uuid.UUID, receivable, payable int64) ([]model.Line, string, error) {
	k := &req.ContactID
	money := model.Line{AccountID: req.AccountID}
	switch req.Type {
	case "talangan":
		// Split bill: I pay the whole amount, my share is my expense, the rest they owe me.
		money.Credit = req.Amount
		lines := []model.Line{{AccountID: receivableID, ContactID: k, Debit: req.Amount - req.MyShare}}
		if req.MyShare > 0 {
			if req.ExpenseAccountID == nil {
				return nil, "", ErrNeedsExpense
			}
			lines = append(lines, model.Line{AccountID: *req.ExpenseAccountID, Debit: req.MyShare})
		}
		return append(lines, money), "loan", nil

	case "pinjam":
		money.Debit = req.Amount
		return []model.Line{money, {AccountID: payableID, ContactID: k, Credit: req.Amount}}, "loan", nil

	case "lunas_masuk", "lunas_keluar":
		in := req.Type == "lunas_masuk"
		owed := payable // what I owe them, settled by paying out
		if in {
			owed = receivable // what they owe me, settled by money coming in
		}
		settle := min(req.Amount, max(owed, 0))
		extra := req.Amount - settle
		if extra > 0 && !req.ConfirmOverpay {
			return nil, "", fmt.Errorf("%w (outstanding %s)", ErrOverpayment, rupiah(max(owed, 0)))
		}

		var lines []model.Line
		if in {
			money.Debit = req.Amount
			lines = append(lines, money)
			if settle > 0 {
				lines = append(lines, model.Line{AccountID: receivableID, ContactID: k, Credit: settle})
			}
			if extra > 0 { // they paid more than they owed: now I owe them the rest
				lines = append(lines, model.Line{AccountID: payableID, ContactID: k, Credit: extra})
			}
		} else {
			if settle > 0 {
				lines = append(lines, model.Line{AccountID: payableID, ContactID: k, Debit: settle})
			}
			if extra > 0 { // I paid more than I owed: now they owe me the rest
				lines = append(lines, model.Line{AccountID: receivableID, ContactID: k, Debit: extra})
			}
			money.Credit = req.Amount
			lines = append(lines, money)
		}
		return lines, "repayment", nil
	}
	return nil, "", fmt.Errorf("unknown loan type %q", req.Type)
}

// PostLoan records money moving between me and a contact.
func (s *LedgerService) PostLoan(ctx context.Context, userID uuid.UUID, req model.LoanRequest) (*model.Entry, error) {
	date, err := time.Parse(time.DateOnly, req.EntryDate)
	if err != nil {
		return nil, err
	}
	c, err := s.contact(ctx, userID, req.ContactID)
	if err != nil {
		return nil, err
	}
	codes, err := s.byCode(ctx, userID)
	if err != nil {
		return nil, err
	}

	// ponytail: balance read outside the insert transaction; one person's book has a single
	// writer, so a race needs two tabs repaying at once. Lock the contact row if phase 5 adds writers.
	lines, kind, err := loanLines(req, codes[codeReceivable].ID, codes[codePayable].ID, c.Receivable, c.Payable)
	if err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, 0, len(lines))
	for _, l := range lines {
		ids = append(ids, l.AccountID)
	}
	accounts, err := s.repo.FindAccounts(ctx, userID, ids)
	if err != nil {
		return nil, err
	}
	if a := accounts[req.AccountID]; a.Type != "Asset" || perContact(a.Code) {
		return nil, fmt.Errorf("%w: money must move through a cash, bank or e-wallet account", ErrWrongAccountType)
	}
	if req.ExpenseAccountID != nil && req.MyShare > 0 && accounts[*req.ExpenseAccountID].Type != "Expense" {
		return nil, fmt.Errorf("%w: your share must go to an expense account", ErrWrongAccountType)
	}

	desc := req.Description
	if desc == "" {
		desc = map[string]string{
			"talangan": "Talangan untuk ", "pinjam": "Pinjam dari ",
			"lunas_masuk": "Pelunasan dari ", "lunas_keluar": "Pelunasan ke ",
		}[req.Type] + c.Name
	}
	e := &model.Entry{EntryDate: date, Kind: kind, Description: desc, Lines: lines}
	if err := s.post(ctx, userID, e, accounts); err != nil {
		return nil, err
	}
	for i := range e.Lines {
		if e.Lines[i].ContactID != nil {
			e.Lines[i].ContactName = c.Name
		}
	}
	return e, nil
}
