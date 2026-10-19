package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/repository"
	"github.com/pozzdol/backend-saluyu/internal/token"
)

var (
	ErrNotFound       = repository.ErrNotFound
	ErrLoanNotPending = repository.ErrLoanNotPending
	ErrEntryLocked    = repository.ErrEntryLocked
	ErrAlreadyLinked  = repository.ErrAlreadyLinked
	ErrNotShared      = errors.New("you no longer share a group with this contact; unlink them first")
	ErrWrongMirror    = errors.New("choose a cash, bank, revenue or expense account for your side")
)

// ShareRepository is what groups and shared loans need from storage.
type ShareRepository interface {
	CreateInviteLink(ctx context.Context, groupID, by uuid.UUID, hash []byte, expires time.Time) error
	AcceptInviteLink(ctx context.Context, hash []byte, userID uuid.UUID) (uuid.UUID, error)
	CreateGroup(ctx context.Context, userID uuid.UUID, name string) (uuid.UUID, error)
	ListGroups(ctx context.Context, userID uuid.UUID) ([]model.Group, error)
	IsActiveMember(ctx context.Context, groupID, userID uuid.UUID) (bool, error)
	ActiveMembers(ctx context.Context, groupID uuid.UUID) ([]model.Member, error)
	SharesActiveGroup(ctx context.Context, a, b uuid.UUID) (bool, error)
	Invite(ctx context.Context, groupID, inviter uuid.UUID, email string) (*model.PendingInvite, error)
	CancelInvite(ctx context.Context, groupID, inviteID uuid.UUID) error
	InvitesForEmail(ctx context.Context, email string) ([]model.InboxInvite, error)
	RespondInvite(ctx context.Context, inviteID, userID uuid.UUID, email string, accept bool) error
	LeaveGroup(ctx context.Context, groupID, userID uuid.UUID) error
	RenameGroup(ctx context.Context, groupID uuid.UUID, name string) error
	LinkContact(ctx context.Context, userID, contactID uuid.UUID, linked *uuid.UUID) error
	PendingLoans(ctx context.Context, userID uuid.UUID) ([]model.InboxLoan, error)
	LoanSource(ctx context.Context, requestID, userID uuid.UUID) (*model.LoanSource, error)
	ContactForUser(ctx context.Context, userID, linked uuid.UUID, name string) (uuid.UUID, error)
	ApproveLoan(ctx context.Context, requestID, userID uuid.UUID, e *model.Entry) (uuid.UUID, error)
	RejectLoan(ctx context.Context, requestID, userID uuid.UUID) error
	PendingChanges(ctx context.Context, userID uuid.UUID) ([]model.InboxChange, error)
	ChangeSource(ctx context.Context, changeID, userID uuid.UUID) (*model.ChangeSource, error)
	ApplyChange(ctx context.Context, changeID, approver, sender uuid.UUID, senderBefore, senderAfter, mirrorBefore, mirrorAfter *model.Entry) error
	RejectChange(ctx context.Context, changeID, userID uuid.UUID) error
}

// ShareService covers phase 5. Every method checks access from the caller's own ID and
// answers ErrNotFound (404) for anything the caller may not see, so it leaks nothing.
type ShareService struct {
	repo    ShareRepository
	ledger  *LedgerService
	reports *ReportService
}

func NewShareService(repo ShareRepository, ledger *LedgerService, reports *ReportService) *ShareService {
	return &ShareService{repo: repo, ledger: ledger, reports: reports}
}

func (s *ShareService) member(ctx context.Context, groupID, userID uuid.UUID) error {
	ok, err := s.repo.IsActiveMember(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

func (s *ShareService) ListGroups(ctx context.Context, userID uuid.UUID) ([]model.Group, error) {
	return s.repo.ListGroups(ctx, userID)
}

func (s *ShareService) CreateGroup(ctx context.Context, userID uuid.UUID, req model.GroupRequest) (*model.Group, error) {
	id, err := s.repo.CreateGroup(ctx, userID, req.Name)
	if err != nil {
		return nil, err
	}
	return s.group(ctx, userID, id)
}

func (s *ShareService) group(ctx context.Context, userID, groupID uuid.UUID) (*model.Group, error) {
	groups, err := s.repo.ListGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].ID == groupID {
			return &groups[i], nil
		}
	}
	return nil, ErrNotFound
}

// Invite answers the same way whether or not the email has an account.
func (s *ShareService) Invite(ctx context.Context, userID, groupID uuid.UUID, req model.InviteRequest) (*model.PendingInvite, error) {
	if err := s.member(ctx, groupID, userID); err != nil {
		return nil, err
	}
	return s.repo.Invite(ctx, groupID, userID, req.Email)
}

func (s *ShareService) CancelInvite(ctx context.Context, userID, groupID, inviteID uuid.UUID) error {
	if err := s.member(ctx, groupID, userID); err != nil {
		return err
	}
	return s.repo.CancelInvite(ctx, groupID, inviteID)
}

func (s *ShareService) Inbox(ctx context.Context, claims *token.Claims) (*model.Inbox, error) {
	invites, err := s.repo.InvitesForEmail(ctx, claims.Email)
	if err != nil {
		return nil, err
	}
	loans, err := s.repo.PendingLoans(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	changes, err := s.repo.PendingChanges(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	return &model.Inbox{Invites: invites, Loans: loans, Changes: changes}, nil
}

// RespondInvite only works on invites sent to the caller's own email.
func (s *ShareService) RespondInvite(ctx context.Context, claims *token.Claims, inviteID uuid.UUID, accept bool) error {
	return s.repo.RespondInvite(ctx, inviteID, claims.UserID, claims.Email, accept)
}

// RenameGroup lets any active member change the name everyone sees.
func (s *ShareService) RenameGroup(ctx context.Context, userID, groupID uuid.UUID, req model.GroupRequest) (*model.Group, error) {
	if err := s.member(ctx, groupID, userID); err != nil {
		return nil, err
	}
	if err := s.repo.RenameGroup(ctx, groupID, req.Name); err != nil {
		return nil, err
	}
	return s.group(ctx, userID, groupID)
}

// LeaveGroup takes effect at once: summaries are checked against active membership on every request.
func (s *ShareService) LeaveGroup(ctx context.Context, userID, groupID uuid.UUID) error {
	return s.repo.LeaveGroup(ctx, groupID, userID)
}

// Summary shows every active member's totals for the payday period containing date
// (each member's own payday). Transactions and descriptions never leave their owner's book.
func (s *ShareService) Summary(ctx context.Context, userID, groupID uuid.UUID, date time.Time) ([]model.MemberSummary, error) {
	if err := s.member(ctx, groupID, userID); err != nil {
		return nil, err
	}
	members, err := s.repo.ActiveMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	out := make([]model.MemberSummary, 0, len(members))
	for _, m := range members {
		// ponytail: one dashboard per member (a few queries each); fine for family-sized groups.
		d, err := s.reports.Dashboard(ctx, m.ID, date)
		if err != nil {
			return nil, err
		}
		sum := model.MemberSummary{Member: m, Period: d.Period, Income: d.Income, Expense: d.Expense,
			Surplus: d.Surplus, SavingsRate: d.SavingsRate, ToSavings: d.ToSavingsTotal}
		for _, b := range d.Budgets {
			if b.Budget > 0 {
				sum.Budget += b.Available
				sum.BudgetSpent += b.Actual
			}
		}
		for _, b := range d.Balances {
			sum.Assets += b.Balance
		}
		out = append(out, sum)
	}
	return out, nil
}

// LinkContact ties a contact to another member, so loans with them get mirrored.
func (s *ShareService) LinkContact(ctx context.Context, userID, contactID uuid.UUID, req model.LinkContactRequest) (*model.Contact, error) {
	if req.UserID != nil {
		ok, err := s.repo.SharesActiveGroup(ctx, userID, *req.UserID)
		if err != nil {
			return nil, err
		}
		if !ok || *req.UserID == userID {
			return nil, ErrNotFound
		}
	}
	if err := s.repo.LinkContact(ctx, userID, contactID, req.UserID); err != nil {
		return nil, err
	}
	return s.ledger.contact(ctx, userID, contactID)
}

var mirrorCode = map[string]string{codeReceivable: codePayable, codePayable: codeReceivable}

// mirrorLines turns the sender's contact lines into the approver's: their receivable is my
// payable and the other way round, with debit and credit swapped. The approver's own account
// takes the balancing side. contactID is the approver's contact for the sender.
func mirrorLines(src []model.Line, ids map[string]uuid.UUID, contactID, own uuid.UUID) []model.Line {
	var lines []model.Line
	var net int64 // debit - credit of the mirrored lines
	for _, l := range src {
		m := model.Line{AccountID: ids[mirrorCode[l.Code]], ContactID: &contactID, Debit: l.Credit, Credit: l.Debit}
		net += m.Debit - m.Credit
		lines = append(lines, m)
	}
	if net > 0 {
		lines = append(lines, model.Line{AccountID: own, Credit: net})
	} else if net < 0 {
		lines = append(lines, model.Line{AccountID: own, Debit: -net})
	}
	return lines
}

// mirrorDescription says what happened from the approver's side. senderNet is the change in
// what the sender is owed: positive means the approver now owes the sender more.
func mirrorDescription(kind string, senderNet int64, name string) string {
	switch {
	case kind == "loan" && senderNet > 0:
		return "Pinjam dari " + name
	case kind == "loan":
		return "Talangan untuk " + name
	case senderNet < 0:
		return "Pelunasan ke " + name
	default:
		return "Pelunasan dari " + name
	}
}

// Approve records the mirror of another member's loan in the caller's book. Approving twice
// (double click, retry) returns the same entry.
func (s *ShareService) Approve(ctx context.Context, userID, requestID uuid.UUID, req model.ApproveLoanRequest) (uuid.UUID, error) {
	src, err := s.repo.LoanSource(ctx, requestID, userID)
	if err != nil {
		return uuid.Nil, err
	}
	switch src.Status {
	case "approved":
		return *src.ToEntryID, nil
	case "rejected":
		return uuid.Nil, ErrLoanNotPending
	}

	codes, err := s.ledger.byCode(ctx, userID)
	if err != nil {
		return uuid.Nil, err
	}
	own, err := s.ledger.repo.FindAccounts(ctx, userID, []uuid.UUID{req.AccountID})
	if err != nil {
		return uuid.Nil, err
	}
	a, ok := own[req.AccountID]
	if !ok {
		return uuid.Nil, ErrAccountNotFound
	}
	if perContact(a.Code) || a.Type == "Equity" || a.Type == "Liability" {
		return uuid.Nil, ErrWrongMirror
	}

	start, err := s.ledger.repo.StartDate(ctx, userID)
	if err != nil {
		return uuid.Nil, err
	}
	if start == nil {
		return uuid.Nil, ErrNotOpened
	}
	date := src.Date
	if date.Before(*start) {
		date = *start // the loan predates my books: it enters them on their first day
	}

	contactID, err := s.repo.ContactForUser(ctx, userID, src.FromUserID, src.FromName)
	if err != nil {
		return uuid.Nil, err
	}
	ids := map[string]uuid.UUID{codeReceivable: codes[codeReceivable].ID, codePayable: codes[codePayable].ID}
	var senderNet int64
	for _, l := range src.Lines {
		senderNet += l.Debit - l.Credit
	}
	e := &model.Entry{
		EntryDate:   date,
		Kind:        src.Kind,
		Description: mirrorDescription(src.Kind, senderNet, src.FromName),
		Lines:       mirrorLines(src.Lines, ids, contactID, a.ID),
	}

	accounts := map[uuid.UUID]model.Account{a.ID: a}
	for _, code := range []string{codeReceivable, codePayable} {
		accounts[codes[code].ID] = codes[code]
	}
	if err := validate(e, accounts); err != nil {
		return uuid.Nil, fmt.Errorf("mirror entry: %w", err)
	}
	if err := s.ledger.checkLock(ctx, userID, e.EntryDate); err != nil {
		return uuid.Nil, err
	}
	return s.repo.ApproveLoan(ctx, requestID, userID, e)
}

func (s *ShareService) Reject(ctx context.Context, userID, requestID uuid.UUID) error {
	return s.repo.RejectLoan(ctx, requestID, userID)
}

// ApproveChange applies another member's edit of a loan to both books at once. My side keeps
// the account I chose when I first approved the loan; approving twice applies it once.
func (s *ShareService) ApproveChange(ctx context.Context, userID, changeID uuid.UUID) error {
	src, err := s.repo.ChangeSource(ctx, changeID, userID)
	if err != nil {
		return err
	}
	switch src.Status {
	case "approved":
		return nil
	case "pending":
	default:
		return ErrLoanNotPending
	}

	senderBefore, err := s.ledger.repo.GetEntry(ctx, src.FromUserID, src.FromEntryID)
	if err != nil {
		return err
	}
	mirrorBefore, err := s.ledger.repo.GetEntry(ctx, userID, src.ToEntryID)
	if err != nil {
		return err
	}

	// The sender's new entry, checked against the sender's own accounts.
	p := src.Proposal.Entry
	date, err := time.Parse(time.DateOnly, p.Date)
	if err != nil {
		return err
	}
	senderAfter := &model.Entry{EntryDate: date, Kind: senderBefore.Kind, Description: p.Description, Lines: p.Lines}
	if p.PeriodDate != nil {
		pd, err := time.Parse(time.DateOnly, *p.PeriodDate)
		if err != nil {
			return err
		}
		senderAfter.PeriodDate = &pd
	}
	if err := s.validateFor(ctx, src.FromUserID, senderAfter); err != nil {
		return err
	}

	// My mirror: same contact and same "my side" account as before, amounts from the proposal.
	var contactID, own uuid.UUID
	for _, l := range mirrorBefore.Lines {
		if l.ContactID != nil {
			contactID = *l.ContactID
		} else {
			own = l.AccountID
		}
	}
	codes, err := s.ledger.byCode(ctx, userID)
	if err != nil {
		return err
	}
	ids := map[string]uuid.UUID{codeReceivable: codes[codeReceivable].ID, codePayable: codes[codePayable].ID}
	mirrorDate := date
	if start, err := s.ledger.repo.StartDate(ctx, userID); err != nil {
		return err
	} else if start != nil && mirrorDate.Before(*start) {
		mirrorDate = *start
	}
	mirrorAfter := &model.Entry{
		EntryDate:   mirrorDate,
		Kind:        mirrorBefore.Kind,
		Description: mirrorDescription(mirrorBefore.Kind, netOf(src.Proposal.MirrorLines), src.FromName),
		Lines:       mirrorLines(src.Proposal.MirrorLines, ids, contactID, own),
	}
	if err := s.validateFor(ctx, userID, mirrorAfter); err != nil {
		return fmt.Errorf("mirror entry: %w", err)
	}
	// Both books change: neither side's closed period may be touched.
	if err := s.ledger.checkLock(ctx, userID, mirrorBefore.EntryDate, mirrorAfter.EntryDate); err != nil {
		return err
	}
	if err := s.ledger.checkLock(ctx, src.FromUserID, senderBefore.EntryDate, senderAfter.EntryDate); err != nil {
		return err
	}
	return s.repo.ApplyChange(ctx, changeID, userID, src.FromUserID, senderBefore, senderAfter, mirrorBefore, mirrorAfter)
}

func (s *ShareService) RejectChange(ctx context.Context, userID, changeID uuid.UUID) error {
	return s.repo.RejectChange(ctx, changeID, userID)
}

// validateFor runs validate() with the accounts of the entry's owner.
func (s *ShareService) validateFor(ctx context.Context, owner uuid.UUID, e *model.Entry) error {
	ids := make([]uuid.UUID, 0, len(e.Lines))
	for _, l := range e.Lines {
		ids = append(ids, l.AccountID)
	}
	accounts, err := s.ledger.repo.FindAccounts(ctx, owner, ids)
	if err != nil {
		return err
	}
	return validate(e, accounts)
}

func netOf(lines []model.Line) (n int64) {
	for _, l := range lines {
		n += l.Debit - l.Credit
	}
	return n
}
