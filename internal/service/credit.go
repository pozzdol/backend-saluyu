package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/installment"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/repository"
)

var (
	ErrNotACard        = repository.ErrNotACard
	ErrPlanNotFound    = repository.ErrPlanNotFound
	ErrAlreadyPaid     = repository.ErrAlreadyPaid
	ErrPlanHasPayments = repository.ErrPlanHasPayments
)

// minPaymentFloor is the smallest minimum payment banks ask for when the bill is larger.
// ponytail: one value for every bank (Rp50.000, the common one); make it per card if needed.
const minPaymentFloor = 5_000_000

type CreditRepository interface {
	FindAccounts(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]model.Account, error)
	ListAccounts(ctx context.Context, userID uuid.UUID) ([]model.Account, error)
	LedgerLines(ctx context.Context, userID, accountID uuid.UUID, from, to *time.Time) (int64, []model.LedgerRow, error)
	NextCode(ctx context.Context, userID uuid.UUID, from, to int) (string, error)
	CreateCard(ctx context.Context, userID uuid.UUID, c *model.Card) error
	UpdateCard(ctx context.Context, userID, accountID uuid.UUID, s model.CardSettings) error
	Cards(ctx context.Context, userID uuid.UUID) ([]model.Card, error)
	CreatePlan(ctx context.Context, userID uuid.UUID, p *model.Plan, borrowing *model.Entry) error
	Plans(ctx context.Context, userID uuid.UUID, id *uuid.UUID) ([]model.Plan, error)
	PayInstallment(ctx context.Context, userID, planID uuid.UUID, seq int, e *model.Entry) error
	DeletePlan(ctx context.Context, userID, planID uuid.UUID) error
}

// CreditService handles credit cards, paylater and installment loans. Entries go through
// the ledger's checks (books opened, dates, balance) before they are stored.
type CreditService struct {
	repo   CreditRepository
	ledger *LedgerService
	now    func() time.Time
}

func NewCreditService(repo CreditRepository, ledger *LedgerService) *CreditService {
	return &CreditService{repo: repo, ledger: ledger, now: time.Now}
}

func (s *CreditService) today() time.Time {
	y, m, d := s.now().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func (s *CreditService) CreateCard(ctx context.Context, userID uuid.UUID, req model.CardRequest) (*model.CardSummary, error) {
	code, err := s.repo.NextCode(ctx, userID, 2201, 2299)
	if err != nil {
		return nil, err
	}
	c := model.Card{
		Account: model.Account{ID: uuid.New(), Code: code, Name: req.Name, Type: "Liability", Category: "Kartu & Paylater",
			NormalBalance: "Credit", IsActive: true, Icon: "credit-card", Color: "indigo", Kind: req.Kind},
		StatementDay: req.StatementDay, DueDay: req.DueDay, CreditLimit: req.CreditLimit, MinPaymentPct: req.MinPaymentPct,
	}
	if err := s.repo.CreateCard(ctx, userID, &c); err != nil {
		return nil, err
	}
	return s.summary(ctx, userID, c, false)
}

func (s *CreditService) UpdateCard(ctx context.Context, userID, id uuid.UUID, req model.CardSettings) (*model.CardSummary, error) {
	if err := s.repo.UpdateCard(ctx, userID, id, req); err != nil {
		return nil, err
	}
	return s.Card(ctx, userID, id)
}

func (s *CreditService) Cards(ctx context.Context, userID uuid.UUID) ([]model.CardSummary, error) {
	cards, err := s.repo.Cards(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]model.CardSummary, 0, len(cards))
	for _, c := range cards {
		sum, err := s.summary(ctx, userID, c, false)
		if err != nil {
			return nil, err
		}
		out = append(out, *sum)
	}
	return out, nil
}

func (s *CreditService) Card(ctx context.Context, userID, id uuid.UUID) (*model.CardSummary, error) {
	cards, err := s.repo.Cards(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, c := range cards {
		if c.Account.ID == id {
			return s.summary(ctx, userID, c, true)
		}
	}
	return nil, ErrNotACard
}

// CycleDates gives the latest statement date on or before today, the one before it, the next
// one, and the due date of the latest bill (the first due_day after its statement date).
func CycleDates(statementDay, dueDay int, today time.Time) (prev, last, next, due time.Time) {
	y, m, d := today.Date()
	last = time.Date(y, m, statementDay, 0, 0, 0, 0, time.UTC)
	if d < statementDay {
		last = last.AddDate(0, -1, 0) // days 1..28 never overflow a month
	}
	prev, next = last.AddDate(0, -1, 0), last.AddDate(0, 1, 0)
	due = time.Date(last.Year(), last.Month(), dueDay, 0, 0, 0, 0, time.UTC)
	if dueDay <= statementDay {
		due = due.AddDate(0, 1, 0)
	}
	return
}

// MinimumPayment is pct% of the bill, at least Rp50.000, never more than the bill.
func MinimumPayment(total int64, pct int) int64 {
	if total <= 0 {
		return 0
	}
	return min(total, max(int64(math.Round(float64(total)*float64(pct)/100)), minPaymentFloor))
}

func (s *CreditService) summary(ctx context.Context, userID uuid.UUID, c model.Card, withLines bool) (*model.CardSummary, error) {
	today := s.today()
	prev, last, next, due := CycleDates(c.StatementDay, c.DueDay, today)
	cycleStart := prev.AddDate(0, 0, 1)
	// One read from the start of the closed cycle to today covers both cycles.
	opening, rows, err := s.repo.LedgerLines(ctx, userID, c.Account.ID, &cycleStart, &today)
	if err != nil {
		return nil, err
	}
	st := model.Statement{Start: cycleStart.Format(time.DateOnly), End: last.Format(time.DateOnly), DueDate: due.Format(time.DateOnly),
		Opening: -opening, Lines: []model.LedgerRow{}}
	cur := model.Cycle{Start: last.AddDate(0, 0, 1).Format(time.DateOnly), End: next.Format(time.DateOnly), Lines: []model.LedgerRow{}}
	for _, r := range rows {
		if r.Date <= st.End {
			st.Charges += r.Credit
			st.Payments += r.Debit
			st.Lines = append(st.Lines, r)
		} else {
			cur.Charges += r.Credit
			cur.Payments += r.Debit
			cur.Lines = append(cur.Lines, r)
		}
	}
	st.Total = st.Opening + st.Charges - st.Payments
	st.Minimum = MinimumPayment(st.Total, c.MinPaymentPct)
	st.PaidSince = cur.Payments
	st.Remaining = max(0, st.Total-st.PaidSince)
	st.MinRemaining = max(0, st.Minimum-st.PaidSince)
	switch {
	case st.Remaining == 0:
		st.Status = "paid"
	case !today.After(due):
		st.Status = "due"
	case st.MinRemaining == 0:
		st.Status = "min_paid" // interest runs on the rest, but the card is not late
	default:
		st.Status = "overdue"
	}
	if !withLines {
		st.Lines, cur.Lines = nil, nil
	}
	owed := st.Total + cur.Charges - cur.Payments
	out := &model.CardSummary{Card: c, Owed: owed, Last: st, Current: cur}
	if c.CreditLimit > 0 {
		avail := c.CreditLimit - owed
		out.Available = &avail
	}
	return out, nil
}

// CreatePlan opens a loan account with its schedule. With ReceivedIntoID it also records the
// money arriving: Dr that account / Cr the loan.
func (s *CreditService) CreatePlan(ctx context.Context, userID uuid.UUID, req model.PlanRequest) (*model.PlanSummary, error) {
	code, err := s.repo.NextCode(ctx, userID, 2301, 2399)
	if err != nil {
		return nil, err
	}
	interest, err := s.interestAccount(ctx, userID, req.InterestAccountID)
	if err != nil {
		return nil, err
	}
	p := model.Plan{
		Account: model.Account{ID: uuid.New(), Code: code, Name: req.Name, Type: "Liability", Category: "Cicilan",
			NormalBalance: "Credit", IsActive: true, Icon: "building-2", Color: "amber", Kind: "loan"},
		InterestAccountID: interest.ID, Principal: req.Principal, AnnualRateBP: int(math.Round(req.AnnualRate * 100)),
		TenorMonths: req.TenorMonths, Method: installment.Method(req.Method), FirstDueDate: req.FirstDueDate,
	}

	var borrowing *model.Entry
	target := req.ReceivedIntoID
	if req.AssetAccountID != nil {
		target = req.AssetAccountID
	}
	if target != nil {
		found, err := s.repo.FindAccounts(ctx, userID, []uuid.UUID{*target})
		if err != nil {
			return nil, err
		}
		into, ok := found[*target]
		if !ok {
			return nil, ErrAccountNotFound
		}
		if req.AssetAccountID != nil && !into.IsFixedAsset() {
			return nil, fmt.Errorf("%w: asset_account_id must be a fixed asset", ErrWrongAccountType)
		}
		if into.Type != "Asset" && into.Type != "Expense" {
			return nil, fmt.Errorf("%w: loan money goes to an asset or pays for an expense", ErrWrongAccountType)
		}
		date := s.today()
		if req.EntryDate != "" {
			date, _ = time.Parse(time.DateOnly, req.EntryDate) // format checked by binding
		}
		borrowing = &model.Entry{EntryDate: date, Kind: "borrowing", Description: req.Name, Lines: []model.Line{
			{AccountID: into.ID, Debit: req.Principal},
			{AccountID: p.Account.ID, Credit: req.Principal},
		}}
		if err := s.ledger.prepare(ctx, userID, borrowing, map[uuid.UUID]model.Account{into.ID: into, p.Account.ID: p.Account}); err != nil {
			return nil, err
		}
	}
	if err := s.repo.CreatePlan(ctx, userID, &p, borrowing); err != nil {
		return nil, err
	}
	return planSummary(p, true), nil
}

// interestAccount is the chosen expense account, or 5111 Bunga Pinjaman.
func (s *CreditService) interestAccount(ctx context.Context, userID uuid.UUID, id *uuid.UUID) (model.Account, error) {
	list, err := s.repo.ListAccounts(ctx, userID)
	if err != nil {
		return model.Account{}, err
	}
	for _, a := range list {
		if (id != nil && a.ID == *id) || (id == nil && a.Code == "5111") {
			if a.Type != "Expense" {
				return a, fmt.Errorf("%w: interest is an expense", ErrWrongAccountType)
			}
			return a, nil
		}
	}
	return model.Account{}, ErrAccountNotFound
}

func planSummary(p model.Plan, withRows bool) *model.PlanSummary {
	first, _ := time.Parse(time.DateOnly, p.FirstDueDate)
	rows := installment.Schedule(p.Principal, p.AnnualRateBP, p.TenorMonths, p.Method, first)
	paid := map[int]model.PaidInstallment{}
	for _, x := range p.Paid {
		paid[x.Seq] = x
	}
	out := &model.PlanSummary{Plan: p, Remaining: p.Principal}
	for i, r := range rows {
		out.TotalInterest += r.Interest
		if x, ok := paid[r.Seq]; ok {
			out.PaidCount++
			out.Remaining -= r.Principal
			if withRows {
				out.Rows = append(out.Rows, model.PlanRow{Row: r, Paid: &x})
			}
			continue
		}
		if out.Next == nil {
			out.Next = &rows[i]
		}
		if withRows {
			out.Rows = append(out.Rows, model.PlanRow{Row: r})
		}
	}
	return out
}

func (s *CreditService) Plans(ctx context.Context, userID uuid.UUID) ([]model.PlanSummary, error) {
	plans, err := s.repo.Plans(ctx, userID, nil)
	if err != nil {
		return nil, err
	}
	out := make([]model.PlanSummary, len(plans))
	for i, p := range plans {
		out[i] = *planSummary(p, false)
	}
	return out, nil
}

func (s *CreditService) Plan(ctx context.Context, userID, id uuid.UUID) (*model.PlanSummary, error) {
	plans, err := s.repo.Plans(ctx, userID, &id)
	if err != nil {
		return nil, err
	}
	if len(plans) == 0 {
		return nil, ErrPlanNotFound
	}
	return planSummary(plans[0], true), nil
}

// PayInstallment pays the earliest unpaid installment: Dr loan (principal) + Dr interest / Cr from.
func (s *CreditService) PayInstallment(ctx context.Context, userID, id uuid.UUID, req model.PayInstallmentRequest) (*model.PlanSummary, error) {
	sum, err := s.Plan(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if sum.Next == nil {
		return nil, ErrAlreadyPaid
	}
	accounts, err := s.repo.FindAccounts(ctx, userID, []uuid.UUID{req.FromAccountID, sum.Account.ID, sum.InterestAccountID})
	if err != nil {
		return nil, err
	}
	from, ok := accounts[req.FromAccountID]
	if !ok {
		return nil, ErrAccountNotFound
	}
	if from.Type != "Asset" {
		return nil, fmt.Errorf("%w: installments are paid from cash, bank or e-wallet", ErrWrongAccountType)
	}
	date, _ := time.Parse(time.DateOnly, req.EntryDate)
	row := *sum.Next
	e := &model.Entry{EntryDate: date, Kind: "installment",
		Description: fmt.Sprintf("%s %d/%d", sum.Account.Name, row.Seq, sum.TenorMonths),
		Lines:       []model.Line{{AccountID: sum.Account.ID, Debit: row.Principal}}}
	if row.Interest > 0 {
		e.Lines = append(e.Lines, model.Line{AccountID: sum.InterestAccountID, Debit: row.Interest})
	}
	e.Lines = append(e.Lines, model.Line{AccountID: from.ID, Credit: row.Payment})
	if err := s.ledger.prepare(ctx, userID, e, accounts); err != nil {
		return nil, err
	}
	if err := s.repo.PayInstallment(ctx, userID, id, row.Seq, e); err != nil {
		return nil, err
	}
	return s.Plan(ctx, userID, id)
}

func (s *CreditService) DeletePlan(ctx context.Context, userID, id uuid.UUID) error {
	return s.repo.DeletePlan(ctx, userID, id)
}
