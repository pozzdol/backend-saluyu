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
	ErrKindChange    = errors.New("the type of a transaction cannot change; delete it and record a new one")
	ErrMirrorLocked  = errors.New("this loan was recorded by the other member; ask them to change the amount")
	ErrWrongEditForm = errors.New("this transaction is edited with another form")
)

// EditResult tells whether an edit is in the books now or waits for the other member.
type EditResult struct {
	Entry   *model.Entry
	Pending bool
}

func (s *LedgerService) entryOfKinds(ctx context.Context, userID, id uuid.UUID, kinds ...string) (*model.Entry, error) {
	e, err := s.repo.GetEntry(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	for _, k := range kinds {
		if e.Kind == k {
			return e, nil
		}
	}
	return nil, fmt.Errorf("%w (%s)", ErrWrongEditForm, e.Kind)
}

// EditEntry changes an income, expense or transfer through the same rules as creating one.
func (s *LedgerService) EditEntry(ctx context.Context, userID, id uuid.UUID, req model.PostEntryRequest) (*EditResult, error) {
	before, err := s.entryOfKinds(ctx, userID, id, "income", "expense", "transfer")
	if err != nil {
		return nil, err
	}
	if req.Kind != before.Kind {
		return nil, ErrKindChange
	}
	next, accounts, err := s.buildSimple(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	return s.replace(ctx, userID, before, next, accounts)
}

// EditLoan changes a loan or repayment. The overpayment check leaves this entry out of the
// balance, so lowering a repayment never trips over its own old amount.
func (s *LedgerService) EditLoan(ctx context.Context, userID, id uuid.UUID, req model.LoanRequest) (*EditResult, error) {
	before, err := s.entryOfKinds(ctx, userID, id, "loan", "repayment")
	if err != nil {
		return nil, err
	}
	next, accounts, c, err := s.buildLoan(ctx, userID, req, before)
	if err != nil {
		return nil, err
	}
	if next.Kind != before.Kind {
		return nil, ErrKindChange
	}
	res, err := s.replace(ctx, userID, before, next, accounts)
	if err == nil && !res.Pending {
		nameContacts(res.Entry, c)
	}
	return res, err
}

// EditLines changes an entry line by line (cash counts and adjustments).
func (s *LedgerService) EditLines(ctx context.Context, userID, id uuid.UUID, req model.LinesRequest) (*EditResult, error) {
	before, err := s.entryOfKinds(ctx, userID, id, "cash_count", "adjustment")
	if err != nil {
		return nil, err
	}
	date, err := time.Parse(time.DateOnly, req.EntryDate)
	if err != nil {
		return nil, err
	}
	next := &model.Entry{EntryDate: date, Kind: before.Kind, Description: req.Description}
	ids := make([]uuid.UUID, 0, len(req.Lines))
	for _, l := range req.Lines {
		next.Lines = append(next.Lines, model.Line{AccountID: l.AccountID, ContactID: l.ContactID, Debit: l.Debit, Credit: l.Credit})
		ids = append(ids, l.AccountID)
	}
	accounts, err := s.repo.FindAccounts(ctx, userID, ids)
	if err != nil {
		return nil, err
	}
	return s.replace(ctx, userID, before, next, accounts)
}

// replace is the single way an entry gets edited, mirroring post(): same date guard, same
// validation, same contact ownership check. A loan the other member already mirrored is not
// changed here: the edit becomes a proposal they approve or reject.
func (s *LedgerService) replace(ctx context.Context, userID uuid.UUID, before, next *model.Entry, accounts map[uuid.UUID]model.Account) (*EditResult, error) {
	start, err := s.repo.StartDate(ctx, userID)
	if err != nil {
		return nil, err
	}
	if start == nil {
		return nil, ErrNotOpened
	}
	if next.EntryDate.Before(*start) {
		return nil, fmt.Errorf("%w (%s)", ErrBeforeStart, start.Format(time.DateOnly))
	}
	if err := validate(next, accounts); err != nil {
		return nil, err
	}
	if err := s.checkContacts(ctx, userID, next); err != nil {
		return nil, err
	}

	status, requestID, sender, other, err := s.repo.LoanLink(ctx, before.ID)
	if err != nil {
		return nil, err
	}
	if status == "approved" {
		if !sender {
			// The mirror may change only its own side (which account I paid from or spent on).
			if !sameContactLines(before, next) {
				return nil, ErrMirrorLocked
			}
		} else {
			mirror, err := s.linesLinkedTo(ctx, userID, next, other, accounts)
			if err != nil {
				return nil, err
			}
			if err := s.repo.ProposeChange(ctx, requestID, userID, model.ChangeProposal{Entry: model.Snapshot(next), MirrorLines: mirror}); err != nil {
				return nil, err
			}
			return &EditResult{Entry: before, Pending: true}, nil
		}
	}

	if err := s.repo.ReplaceEntry(ctx, userID, before, next); err != nil {
		return nil, err
	}
	return &EditResult{Entry: next}, nil
}

// sameContactLines reports whether two entries owe and are owed the same amounts per person.
func sameContactLines(a, b *model.Entry) bool {
	net := func(e *model.Entry) map[string]int64 {
		m := map[string]int64{}
		for _, l := range e.Lines {
			if l.ContactID != nil {
				m[l.ContactID.String()+"/"+l.AccountID.String()] += l.Debit - l.Credit
			}
		}
		return m
	}
	x, y := net(a), net(b)
	if len(x) != len(y) {
		return false
	}
	for k, v := range x {
		if y[k] != v {
			return false
		}
	}
	return true
}

// linesLinkedTo returns the entry's lines on contacts linked to user other, with account codes.
func (s *LedgerService) linesLinkedTo(ctx context.Context, userID uuid.UUID, e *model.Entry, other uuid.UUID, accounts map[uuid.UUID]model.Account) ([]model.Line, error) {
	contacts, err := s.repo.ListContacts(ctx, userID, nil)
	if err != nil {
		return nil, err
	}
	linked := map[uuid.UUID]bool{}
	for _, c := range contacts {
		if c.LinkedUserID != nil && *c.LinkedUserID == other {
			linked[c.ID] = true
		}
	}
	var out []model.Line
	for _, l := range e.Lines {
		if l.ContactID != nil && linked[*l.ContactID] {
			l.Code = accounts[l.AccountID].Code
			out = append(out, l)
		}
	}
	return out, nil
}

// EntryDetail returns an entry with its revisions and its shared-loan state.
func (s *LedgerService) EntryDetail(ctx context.Context, userID, id uuid.UUID) (*model.EntryDetail, error) {
	e, err := s.repo.GetEntry(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	revs, err := s.repo.Revisions(ctx, id)
	if err != nil {
		return nil, err
	}
	status, requestID, sender, _, err := s.repo.LoanLink(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &model.EntryDetail{Entry: *e, Revisions: revs, LoanStatus: status, Mirrored: status != "" && !sender}
	if status == "approved" && sender {
		if d.ChangePending, err = s.repo.ChangePending(ctx, requestID); err != nil {
			return nil, err
		}
	}
	return d, nil
}
