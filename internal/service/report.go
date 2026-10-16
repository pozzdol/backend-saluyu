package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/period"
)

var ErrNotExpenseAccount = errors.New("budgets are only for expense accounts")

// ReportRepository is what reports need from storage. *repository.LedgerRepository implements it.
type ReportRepository interface {
	Exporter
	ListAccounts(ctx context.Context, userID uuid.UUID) ([]model.Account, error)
	FindAccounts(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]model.Account, error)
	PeriodStartDay(ctx context.Context, userID uuid.UUID) (int, error)
	Movements(ctx context.Context, userID uuid.UUID, from, to time.Time, byPeriodDate bool) ([]model.Movement, error)
	EffectiveBudgets(ctx context.Context, userID uuid.UUID, periodStart time.Time) ([]model.Budget, error)
	UpsertBudget(ctx context.Context, userID, accountID uuid.UUID, periodStart time.Time, amount int64) error
	EntryLines(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]model.EntryLine, error)
}

// ReportService computes every figure from journal lines on each request; nothing is stored.
type ReportService struct {
	repo ReportRepository
	now  func() time.Time
}

func NewReportService(repo ReportRepository) *ReportService {
	return &ReportService{repo: repo, now: time.Now}
}

// periodOf is the user's payday period containing date.
func (s *ReportService) periodOf(ctx context.Context, userID uuid.UUID, date time.Time) (period.Period, error) {
	day, err := s.repo.PeriodStartDay(ctx, userID)
	if err != nil {
		return period.Period{}, err
	}
	return period.For(date, day), nil
}

func periodJSON(p period.Period) model.PeriodJSON {
	return model.PeriodJSON{
		Start: p.Start.Format(time.DateOnly),
		End:   p.End.Format(time.DateOnly),
		Next:  p.Next.Format(time.DateOnly),
		Prev:  p.Start.AddDate(0, 0, -1).Format(time.DateOnly),
		Label: p.Label(),
	}
}

// amounts sums the movements keep accepts per account, on the account's normal side
// (revenue and liabilities count credits as positive). Zero rows are dropped; order follows
// the movements, which come sorted by account code.
func amounts(moves []model.Movement, keep func(model.Movement) bool) ([]model.AmountRow, int64) {
	rows := []model.AmountRow{}
	at := map[uuid.UUID]int{}
	var total int64
	for _, m := range moves {
		if !keep(m) {
			continue
		}
		amount := m.Debit - m.Credit
		if model.NormalBalance(m.Type) == "Credit" {
			amount = -amount
		}
		i, ok := at[m.AccountID]
		if !ok {
			i = len(rows)
			at[m.AccountID] = i
			rows = append(rows, model.AmountRow{AccountID: m.AccountID, Code: m.Code, Name: m.Name})
		}
		rows[i].Amount += amount
		total += amount
	}
	nonZero := rows[:0]
	for _, r := range rows {
		if r.Amount != 0 {
			nonZero = append(nonZero, r)
		}
	}
	return nonZero, total
}

func ofType(t string) func(model.Movement) bool {
	return func(m model.Movement) bool { return m.Type == t }
}

// budgetRows compares each account's effective budget with its spending in p.
// moves must be the period movements of p.
func (s *ReportService) budgetRows(ctx context.Context, userID uuid.UUID, p period.Period, accounts []model.Account, moves []model.Movement) ([]model.BudgetRow, error) {
	budgets, err := s.repo.EffectiveBudgets(ctx, userID, p.Start)
	if err != nil {
		return nil, err
	}
	byAccount := make(map[uuid.UUID]model.Budget, len(budgets))
	for _, b := range budgets {
		byAccount[b.AccountID] = b
	}
	actual := map[uuid.UUID]int64{}
	for _, m := range moves {
		actual[m.AccountID] += m.Debit - m.Credit
	}

	rows := []model.BudgetRow{}
	for _, a := range accounts {
		b, ok := byAccount[a.ID]
		row := model.BudgetRow{AccountID: a.ID, Name: a.Name, Budget: b.Amount, Actual: actual[a.ID],
			Inherited: ok && b.PeriodStart.Before(p.Start)}
		row.Remaining = row.Budget - row.Actual
		row.Over = row.Actual > row.Budget
		rows = append(rows, row)
	}
	return rows, nil
}

// activeOf keeps the active accounts of type t, in the given (code) order.
func activeOf(accounts []model.Account, t string) []model.Account {
	var out []model.Account
	for _, a := range accounts {
		if a.Type == t && a.IsActive {
			out = append(out, a)
		}
	}
	return out
}

func (s *ReportService) Dashboard(ctx context.Context, userID uuid.UUID, date time.Time) (*model.Dashboard, error) {
	p, err := s.periodOf(ctx, userID, date)
	if err != nil {
		return nil, err
	}
	accounts, err := s.repo.ListAccounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	moves, err := s.repo.Movements(ctx, userID, p.Start, p.Next, true)
	if err != nil {
		return nil, err
	}
	// Balances use the real bank date, up to and including the period's last day.
	held, err := s.repo.Movements(ctx, userID, time.Time{}, p.Next, false)
	if err != nil {
		return nil, err
	}

	d := &model.Dashboard{Period: periodJSON(p)}
	_, d.Income = amounts(moves, ofType("Revenue"))
	_, d.Expense = amounts(moves, ofType("Expense"))
	d.Surplus = d.Income - d.Expense
	if d.Income > 0 {
		d.SavingsRate = float64(d.Surplus) / float64(d.Income)
	}
	d.ToSavings, d.ToSavingsTotal = amounts(moves, func(m model.Movement) bool {
		return m.Type == "Asset" && m.Kind == "transfer" && (m.Category == "Tabungan" || m.Category == "Investasi")
	})

	if d.Budgets, err = s.budgetRows(ctx, userID, p, activeOf(accounts, "Expense"), moves); err != nil {
		return nil, err
	}

	balance := map[uuid.UUID]int64{}
	for _, m := range held {
		balance[m.AccountID] += m.Debit - m.Credit
	}
	d.Balances = []model.BalanceRow{}
	for _, a := range activeOf(accounts, "Asset") {
		d.Balances = append(d.Balances, model.BalanceRow{AccountID: a.ID, Code: a.Code, Name: a.Name,
			Category: a.Category, Balance: balance[a.ID]})
	}
	return d, nil
}

func (s *ReportService) Budgets(ctx context.Context, userID uuid.UUID, date time.Time) (*model.BudgetList, error) {
	p, err := s.periodOf(ctx, userID, date)
	if err != nil {
		return nil, err
	}
	accounts, err := s.repo.ListAccounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	moves, err := s.repo.Movements(ctx, userID, p.Start, p.Next, true)
	if err != nil {
		return nil, err
	}
	rows, err := s.budgetRows(ctx, userID, p, activeOf(accounts, "Expense"), moves)
	if err != nil {
		return nil, err
	}
	return &model.BudgetList{Period: periodJSON(p), Rows: rows}, nil
}

// SetBudget sets the budget from the start of the period containing date onward.
func (s *ReportService) SetBudget(ctx context.Context, userID, accountID uuid.UUID, date time.Time, amount int64) (*model.BudgetRow, error) {
	found, err := s.repo.FindAccounts(ctx, userID, []uuid.UUID{accountID})
	if err != nil {
		return nil, err
	}
	a, ok := found[accountID]
	if !ok {
		return nil, ErrAccountNotFound
	}
	if a.Type != "Expense" {
		return nil, ErrNotExpenseAccount
	}
	p, err := s.periodOf(ctx, userID, date)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpsertBudget(ctx, userID, accountID, p.Start, amount); err != nil {
		return nil, err
	}
	moves, err := s.repo.Movements(ctx, userID, p.Start, p.Next, true)
	if err != nil {
		return nil, err
	}
	rows, err := s.budgetRows(ctx, userID, p, []model.Account{a}, moves)
	if err != nil {
		return nil, err
	}
	return &rows[0], nil
}

// IncomeStatement covers the period containing date; withCompare adds the previous period
// and the average of the three before it, per row.
func (s *ReportService) IncomeStatement(ctx context.Context, userID uuid.UUID, date time.Time, withCompare bool) (*model.IncomeStatement, error) {
	p, err := s.periodOf(ctx, userID, date)
	if err != nil {
		return nil, err
	}
	moves, err := s.repo.Movements(ctx, userID, p.Start, p.Next, true)
	if err != nil {
		return nil, err
	}
	is := &model.IncomeStatement{Period: periodJSON(p)}
	is.Revenue, is.RevenueTotal = amounts(moves, ofType("Revenue"))
	is.Expense, is.ExpenseTotal = amounts(moves, ofType("Expense"))
	is.NetIncome = is.RevenueTotal - is.ExpenseTotal
	if withCompare {
		if err := s.compare(ctx, userID, p, is); err != nil {
			return nil, err
		}
	}
	return is, nil
}

// BalanceSheet is the position on asOf by real entry dates. Revenue and expense are never
// closed into equity, so their running difference shows as current earnings.
func (s *ReportService) BalanceSheet(ctx context.Context, userID uuid.UUID, asOf time.Time) (*model.BalanceSheet, error) {
	moves, err := s.repo.Movements(ctx, userID, time.Time{}, asOf.AddDate(0, 0, 1), false)
	if err != nil {
		return nil, err
	}
	bs := &model.BalanceSheet{AsOf: asOf.Format(time.DateOnly)}
	bs.Assets, bs.AssetsTotal = amounts(moves, ofType("Asset"))
	bs.Liabilities, bs.LiabilitiesTotal = amounts(moves, ofType("Liability"))
	bs.Equity, bs.EquityTotal = amounts(moves, ofType("Equity"))
	_, revenue := amounts(moves, ofType("Revenue"))
	_, expense := amounts(moves, ofType("Expense"))
	bs.CurrentEarnings = revenue - expense
	bs.Balanced = bs.AssetsTotal == bs.LiabilitiesTotal+bs.EquityTotal+bs.CurrentEarnings
	return bs, nil
}
