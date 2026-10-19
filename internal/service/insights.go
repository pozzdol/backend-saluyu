package service

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/period"
)

// isCash: money that can be spent now. Receivables wait on other people and investments
// must be sold first, so both stay out, and so do vehicles and property.
func isCash(code, typ, category string) bool {
	return typ == "Asset" && code != "1101" && category != "Investasi" && category != model.FixedAssetCategory
}

// isSpendable: cash for day-to-day spending. Savings accounts (Tabungan) are money put
// aside, so safe-to-spend leaves them out too.
func isSpendable(code, typ, category string) bool {
	return isCash(code, typ, category) && category != "Tabungan"
}

func (s *ReportService) today() time.Time {
	y, m, d := s.now().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func byAccount(rows []model.AmountRow) map[uuid.UUID]int64 {
	out := make(map[uuid.UUID]int64, len(rows))
	for _, r := range rows {
		out[r.AccountID] = r.Amount
	}
	return out
}

func (s *ReportService) periodMoves(ctx context.Context, userID uuid.UUID, p period.Period) ([]model.Movement, error) {
	return s.repo.Movements(ctx, userID, p.Start, p.Next, true)
}

// Insights is the top of the dashboard: safe spending per day, where the money went, the
// days of the period, a few observations and the budgets running out.
func (s *ReportService) Insights(ctx context.Context, userID uuid.UUID, date time.Time) (*model.Insights, error) {
	cfg, err := s.repo.PeriodConfig(ctx, userID)
	if err != nil {
		return nil, err
	}
	p := period.For(date, cfg)
	accounts, err := s.repo.ListAccounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	moves, err := s.periodMoves(ctx, userID, p)
	if err != nil {
		return nil, err
	}
	out := &model.Insights{Period: periodJSON(p), Insights: []model.Insight{}, BudgetAlerts: []model.BudgetAlert{}}
	out.ByCategory, out.ExpenseTotal = amounts(moves, ofType("Expense"))
	opening, income, err := s.periodMoney(ctx, userID, p, moves)
	if err != nil {
		return nil, err
	}

	budgets, err := s.budgetRows(ctx, userID, p, activeOf(accounts, "Expense"), moves)
	if err != nil {
		return nil, err
	}
	var unspent int64
	budgeted := false
	for _, b := range budgets {
		if b.Budget <= 0 {
			continue
		}
		budgeted = true
		unspent += max(b.Remaining, 0)
		switch {
		case b.Actual > b.Available:
			out.BudgetAlerts = append(out.BudgetAlerts, model.BudgetAlert{AccountID: b.AccountID, Name: b.Name, Budget: b.Available, Actual: b.Actual, Level: "over"})
		case b.Actual*10 >= b.Available*8:
			out.BudgetAlerts = append(out.BudgetAlerts, model.BudgetAlert{AccountID: b.AccountID, Name: b.Name, Budget: b.Available, Actual: b.Actual, Level: "warn"})
		}
	}
	today := s.today()
	safe, err := s.safeMoney(ctx, userID, p, accounts, today)
	if err != nil {
		return nil, err
	}
	safe.OpeningCash, safe.PeriodMoney = opening, opening+income
	safe.Deficit = out.ExpenseTotal > safe.PeriodMoney
	if budgeted {
		// Unspent budgets, but never more than the money that is actually there.
		safe.Basis, safe.Remaining = "budget", min(unspent, safe.Remaining)
	}
	switch {
	case today.After(p.End):
		safe.DaysLeft = 0
	case today.Before(p.Start):
		safe.DaysLeft = int(p.Next.Sub(p.Start).Hours() / 24)
	default:
		safe.DaysLeft = int(p.Next.Sub(today).Hours() / 24)
	}
	if safe.DaysLeft > 0 {
		safe.Amount = safe.Remaining / int64(safe.DaysLeft)
	}
	out.SafePerDay = safe

	lines, err := s.repo.EntryLines(ctx, userID, p.Start, p.Next)
	if err != nil {
		return nil, err
	}
	daily := map[string]*model.DayTotal{}
	for d := p.Start; d.Before(p.Next); d = d.AddDate(0, 0, 1) {
		k := d.Format(time.DateOnly)
		daily[k] = &model.DayTotal{Date: k}
		out.Daily = append(out.Daily, model.DayTotal{Date: k})
	}
	for _, l := range lines {
		t := daily[l.Date.Format(time.DateOnly)]
		switch l.Type {
		case "Expense":
			t.Expense += l.Debit - l.Credit
		case "Revenue":
			t.Income += l.Credit - l.Debit
		}
	}
	for i := range out.Daily {
		out.Daily[i] = *daily[out.Daily[i].Date]
	}

	if out.Insights, err = s.observations(ctx, userID, p, cfg, out.ByCategory, income, out.ExpenseTotal); err != nil {
		return nil, err
	}
	return out, nil
}

// periodMoney is the spendable cash the period opened with and the income earned in it.
// Opening balances entered inside the period count as cash at its start. Both go by the
// period date, so salary paid early for this period counts once, as this period's income.
// moves must be the period movements of p.
func (s *ReportService) periodMoney(ctx context.Context, userID uuid.UUID, p period.Period, moves []model.Movement) (opening, income int64, err error) {
	before, err := s.repo.Movements(ctx, userID, time.Time{}, p.Start, true)
	if err != nil {
		return 0, 0, err
	}
	for _, m := range before {
		if isSpendable(m.Code, m.Type, m.Category) {
			opening += m.Debit - m.Credit
		}
	}
	for _, m := range moves {
		if m.Kind == "opening" && isSpendable(m.Code, m.Type, m.Category) {
			opening += m.Debit - m.Credit
		}
	}
	_, income = amounts(moves, ofType("Revenue"))
	return opening, income, nil
}

// safeMoney is the spendable cash on hand (as of today, inside p) minus what is already
// promised: card debt, and installments and recurring payments due by the period's end.
// Going by cash instead of "period money minus expenses" keeps it steady when a bill is
// paid (cash and the bill drop together) and counts a card purchase once, as card debt.
func (s *ReportService) safeMoney(ctx context.Context, userID uuid.UUID, p period.Period, accounts []model.Account, today time.Time) (model.SafePerDay, error) {
	safe := model.SafePerDay{Basis: "period_money"}
	upTo := today.AddDate(0, 0, 1)
	if upTo.After(p.Next) {
		upTo = p.Next
	} else if upTo.Before(p.Start) {
		upTo = p.Start
	}
	held, err := s.repo.Movements(ctx, userID, time.Time{}, upTo, true)
	if err != nil {
		return safe, err
	}
	card := map[uuid.UUID]bool{}
	for _, a := range accounts {
		card[a.ID] = a.IsCard()
	}
	var owed int64
	for _, m := range held {
		if isSpendable(m.Code, m.Type, m.Category) {
			safe.Cash += m.Debit - m.Credit
		}
		if card[m.AccountID] {
			owed += m.Credit - m.Debit
		}
	}
	due, err := s.scheduled(ctx, userID, p.End, accounts)
	if err != nil {
		return safe, err
	}
	safe.Scheduled = max(owed, 0) + due
	safe.Remaining = max(safe.Cash-safe.Scheduled, 0)
	return safe, nil
}

// scheduled sums unpaid installments and open recurring payments (overdue ones included)
// due on or before until. Only rules that take money out count: expenses, and transfers
// from spendable cash to somewhere else. Transfers to a card or loan are left out because
// the card debt and the installments are counted already.
// ponytail: income rules are not added back; a salary that is only scheduled is not money yet.
func (s *ReportService) scheduled(ctx context.Context, userID uuid.UUID, until time.Time, accounts []model.Account) (int64, error) {
	var total int64
	plans, err := s.repo.Plans(ctx, userID, nil)
	if err != nil {
		return 0, err
	}
	for _, p := range plans {
		for _, r := range planSummary(p, true).Rows {
			if r.Paid == nil && !r.DueDate.After(until) {
				total += r.Payment
			}
		}
	}
	if s.recurring == nil {
		return total, nil
	}
	due, err := s.recurring.Upcoming(ctx, userID, until)
	if err != nil {
		return 0, err
	}
	byID := map[uuid.UUID]model.Account{}
	for _, a := range accounts {
		byID[a.ID] = a
	}
	for _, o := range due {
		from, to := byID[o.Rule.FromAccountID], byID[o.Rule.ToAccountID]
		if o.Rule.Kind == "expense" || o.Rule.Kind == "transfer" && isSpendable(from.Code, from.Type, from.Category) &&
			!isSpendable(to.Code, to.Type, to.Category) && to.Kind != "loan" && !to.IsCard() {
			total += o.Rule.Amount
		}
	}
	return total, nil
}

const insightFloor = 5_000_000 // Rp50.000: smaller changes are noise

// observations compares this period with the last one and the average of the three before.
func (s *ReportService) observations(ctx context.Context, userID uuid.UUID, p period.Period, cfg period.Config, cats []model.AmountRow, income, expense int64) ([]model.Insight, error) {
	prevMoves, err := s.periodMoves(ctx, userID, period.Back(p, 1, cfg))
	if err != nil {
		return nil, err
	}
	prevCats, prevExpense := amounts(prevMoves, ofType("Expense"))
	_, prevIncome := amounts(prevMoves, ofType("Revenue"))
	prev := byAccount(prevCats)
	avg := map[uuid.UUID]int64{}
	for k := 1; k <= 3; k++ {
		m, err := s.periodMoves(ctx, userID, period.Back(p, k, cfg))
		if err != nil {
			return nil, err
		}
		rows, _ := amounts(m, ofType("Expense"))
		for _, r := range rows {
			avg[r.AccountID] += r.Amount
		}
	}

	var out []model.Insight
	pct := func(diff, base int64) float64 { return math.Round(float64(diff)/float64(base)*1000) / 10 }
	seen := map[uuid.UUID]bool{}
	for _, c := range cats {
		id := c.AccountID
		if before := prev[id]; before > 0 && c.Amount-before >= insightFloor && (c.Amount-before)*5 >= before {
			out = append(out, model.Insight{Kind: "category_up", AccountID: &id, Name: c.Name, Amount: c.Amount - before, Percent: pct(c.Amount-before, before)})
			seen[id] = true
		}
	}
	names := map[uuid.UUID]string{}
	for _, c := range cats {
		names[c.AccountID] = c.Name
	}
	for _, c := range prevCats {
		names[c.AccountID] = c.Name
	}
	this := byAccount(cats)
	for id, total := range avg {
		mean := total / 3
		if seen[id] || mean <= 0 {
			continue
		}
		if saved := mean - this[id]; saved >= insightFloor {
			id := id
			out = append(out, model.Insight{Kind: "category_down", AccountID: &id, Name: names[id], Amount: saved, Percent: pct(saved, mean)})
		}
	}
	if income > 0 && prevIncome > 0 {
		now, before := float64(income-expense)/float64(income), float64(prevIncome-prevExpense)/float64(prevIncome)
		if diff := now - before; math.Abs(diff) >= 0.05 {
			kind := "savings_up"
			if diff < 0 {
				kind = "savings_down"
			}
			out = append(out, model.Insight{Kind: kind, Amount: abs64((income - expense) - (prevIncome - prevExpense)), Percent: math.Round(diff*1000) / 10})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Amount > out[j].Amount })
	if len(out) > 3 {
		out = out[:3]
	}
	if out == nil {
		out = []model.Insight{}
	}
	return out, nil
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// netWorth is assets minus liabilities from balance movements.
func netWorth(moves []model.Movement) int64 {
	_, assets := amounts(moves, ofType("Asset"))
	_, liabilities := amounts(moves, ofType("Liability"))
	return assets - liabilities
}

// Trends returns n periods ending with the one containing date.
// ponytail: two queries per period (n=12 is 24 small grouped queries); one windowed query if it gets slow.
func (s *ReportService) Trends(ctx context.Context, userID uuid.UUID, date time.Time, n int) ([]model.TrendPoint, error) {
	cfg, err := s.repo.PeriodConfig(ctx, userID)
	if err != nil {
		return nil, err
	}
	p := period.For(date, cfg)
	out := make([]model.TrendPoint, 0, n)
	for k := n - 1; k >= 0; k-- {
		q := period.Back(p, k, cfg)
		moves, err := s.periodMoves(ctx, userID, q)
		if err != nil {
			return nil, err
		}
		held, err := s.repo.Movements(ctx, userID, time.Time{}, q.Next, false)
		if err != nil {
			return nil, err
		}
		pt := model.TrendPoint{Period: periodJSON(q), NetWorth: netWorth(held)}
		pt.ByCategory, pt.Expense = amounts(moves, ofType("Expense"))
		_, pt.Income = amounts(moves, ofType("Revenue"))
		out = append(out, pt)
	}
	return out, nil
}

func judge(value, good, warn float64, lowerIsOK bool) model.Ratio {
	r := model.Ratio{Value: math.Round(value*1000) / 1000, Good: good, Warn: warn, LowerIsOK: lowerIsOK, Status: "bad"}
	switch {
	case lowerIsOK && value <= good, !lowerIsOK && value >= good:
		r.Status = "good"
	case lowerIsOK && value <= warn, !lowerIsOK && value >= warn:
		r.Status = "warn"
	}
	return r
}

// Health judges the last 12 complete periods (fewer while the books are young).
func (s *ReportService) Health(ctx context.Context, userID uuid.UUID, date time.Time) (*model.Health, error) {
	cfg, err := s.repo.PeriodConfig(ctx, userID)
	if err != nil {
		return nil, err
	}
	p := period.For(date, cfg)
	type totals struct{ income, expense, debt int64 }
	var past []totals // most recent first, only periods with activity
	for k := 1; k <= 12; k++ {
		moves, err := s.periodMoves(ctx, userID, period.Back(p, k, cfg))
		if err != nil {
			return nil, err
		}
		var t totals
		_, t.income = amounts(moves, ofType("Revenue"))
		_, t.expense = amounts(moves, ofType("Expense"))
		for _, m := range moves {
			// Paying down a loan or card; debts to people (2101) are not loans with a schedule.
			if m.Type == "Liability" && m.Code != "2101" && (m.Kind == "installment" || m.Kind == "transfer") {
				t.debt += m.Debit
			}
		}
		if t.income != 0 || t.expense != 0 {
			past = append(past, t)
		}
	}
	held, err := s.repo.Movements(ctx, userID, time.Time{}, s.today().AddDate(0, 0, 1), false)
	if err != nil {
		return nil, err
	}
	h := &model.Health{Periods: len(past)}
	for _, m := range held {
		if isCash(m.Code, m.Type, m.Category) {
			h.Cash += m.Debit - m.Credit
		}
	}
	none := func(good, warn float64, lower bool) model.Ratio {
		return model.Ratio{Good: good, Warn: warn, LowerIsOK: lower, Status: "none"}
	}
	h.EmergencyMonths, h.DebtRatio = none(6, 3, false), none(0.3, 0.4, true)
	h.Savings3, h.Savings6, h.Savings12 = none(0.2, 0.1, false), none(0.2, 0.1, false), none(0.2, 0.1, false)
	if len(past) == 0 {
		return h, nil
	}
	sum := func(n int) (in, ex, debt int64, used int) {
		for i := 0; i < n && i < len(past); i++ {
			in, ex, debt, used = in+past[i].income, ex+past[i].expense, debt+past[i].debt, used+1
		}
		return
	}
	in3, ex3, debt3, n3 := sum(3)
	perPeriod := func(total int64) int64 { return total / int64(n3) / 100 * 100 } // whole rupiah
	h.AvgIncome, h.AvgExpense, h.AvgDebtPayments = perPeriod(in3), perPeriod(ex3), perPeriod(debt3)
	if h.AvgExpense > 0 {
		h.EmergencyMonths = judge(float64(h.Cash)/float64(h.AvgExpense), 6, 3, false)
	}
	if h.AvgIncome > 0 {
		h.DebtRatio = judge(float64(h.AvgDebtPayments)/float64(h.AvgIncome), 0.3, 0.4, true)
	}
	for _, x := range []struct {
		n   int
		dst *model.Ratio
	}{{3, &h.Savings3}, {6, &h.Savings6}, {12, &h.Savings12}} {
		if in, ex, _, used := sum(x.n); in > 0 && used == x.n {
			*x.dst = judge(float64(in-ex)/float64(in), 0.2, 0.1, false)
		}
	}
	return h, nil
}

// CashFlow splits the period's change in cash by what the money came from or went to.
func (s *ReportService) CashFlow(ctx context.Context, userID uuid.UUID, date time.Time) (*model.CashFlow, error) {
	p, err := s.periodOf(ctx, userID, date)
	if err != nil {
		return nil, err
	}
	before, err := s.repo.Movements(ctx, userID, time.Time{}, p.Start, false)
	if err != nil {
		return nil, err
	}
	cf := &model.CashFlow{Period: periodJSON(p)}
	for _, m := range before {
		if isCash(m.Code, m.Type, m.Category) {
			cf.Beginning += m.Debit - m.Credit
		}
	}
	lines, err := s.repo.EntryLines(ctx, userID, p.Start, p.Next)
	if err != nil {
		return nil, err
	}
	type key struct {
		section string
		id      uuid.UUID
	}
	sums := map[key]*model.AmountRow{}
	var order []key
	// Every non-cash line of an entry that moved cash is the other side of that cash:
	// its debit minus credit, negated, is the cash it brought in.
	for i := 0; i < len(lines); {
		j := i
		var cash int64
		for ; j < len(lines) && lines[j].EntryID == lines[i].EntryID; j++ {
			if l := lines[j]; isCash(l.Code, l.Type, l.Category) {
				cash += l.Debit - l.Credit
			}
		}
		touches := false
		for _, l := range lines[i:j] {
			touches = touches || isCash(l.Code, l.Type, l.Category)
		}
		if touches {
			for _, l := range lines[i:j] {
				if isCash(l.Code, l.Type, l.Category) {
					continue
				}
				section := map[string]string{"Revenue": "operating", "Expense": "operating", "Asset": "investing",
					"Liability": "financing", "Equity": "opening"}[l.Type]
				k := key{section, l.AccountID}
				row, ok := sums[k]
				if !ok {
					row = &model.AmountRow{AccountID: l.AccountID, Code: l.Code, Name: l.Name}
					sums[k] = row
					order = append(order, k)
				}
				row.Amount -= l.Debit - l.Credit
			}
		}
		cf.Net += cash
		i = j
	}
	sort.SliceStable(order, func(a, b int) bool { return sums[order[a]].Code < sums[order[b]].Code })
	sections := map[string]*model.CashFlowSection{"operating": &cf.Operating, "investing": &cf.Investing, "financing": &cf.Financing}
	for _, sec := range sections {
		sec.Rows = []model.AmountRow{}
	}
	for _, k := range order {
		row := *sums[k]
		if row.Amount == 0 {
			continue
		}
		if k.section == "opening" {
			cf.Opening += row.Amount
			continue
		}
		sections[k.section].Rows = append(sections[k.section].Rows, row)
		sections[k.section].Total += row.Amount
	}
	cf.Ending = cf.Beginning + cf.Net
	return cf, nil
}

// TaxSummary is what the annual tax return (SPT) asks for: holdings and debts on
// 31 December, and the year's income by source.
func (s *ReportService) TaxSummary(ctx context.Context, userID uuid.UUID, year int) (*model.TaxSummary, error) {
	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	next := start.AddDate(1, 0, 0)
	held, err := s.repo.Movements(ctx, userID, time.Time{}, next, false)
	if err != nil {
		return nil, err
	}
	moves, err := s.repo.Movements(ctx, userID, start, next, false)
	if err != nil {
		return nil, err
	}
	t := &model.TaxSummary{Year: year, AsOf: next.AddDate(0, 0, -1).Format(time.DateOnly)}
	t.Assets, t.AssetsTotal = amounts(held, ofType("Asset"))
	t.Liabilities, t.LiabilitiesTotal = amounts(held, ofType("Liability"))
	t.Income, t.IncomeTotal = amounts(moves, ofType("Revenue"))
	return t, nil
}

// compare fills Prev and Avg3 on income statement rows and returns the compared totals.
func (s *ReportService) compare(ctx context.Context, userID uuid.UUID, p period.Period, is *model.IncomeStatement) error {
	cfg, err := s.repo.PeriodConfig(ctx, userID)
	if err != nil {
		return err
	}
	var prev [2]map[uuid.UUID]int64 // revenue, expense of the previous period
	var sum [2]map[uuid.UUID]int64  // over the three before
	var prevTotal, sumTotal [2]int64
	for i := range prev {
		prev[i], sum[i] = map[uuid.UUID]int64{}, map[uuid.UUID]int64{}
	}
	for k := 1; k <= 3; k++ {
		moves, err := s.periodMoves(ctx, userID, period.Back(p, k, cfg))
		if err != nil {
			return err
		}
		for i, t := range []string{"Revenue", "Expense"} {
			rows, total := amounts(moves, ofType(t))
			for _, r := range rows {
				sum[i][r.AccountID] += r.Amount
				if k == 1 {
					prev[i][r.AccountID] = r.Amount
				}
			}
			sumTotal[i] += total
			if k == 1 {
				prevTotal[i] = total
			}
		}
	}
	for i, rows := range [][]model.AmountRow{is.Revenue, is.Expense} {
		for j := range rows {
			pv, av := prev[i][rows[j].AccountID], sum[i][rows[j].AccountID]/3/100*100 // whole rupiah
			rows[j].Prev, rows[j].Avg3 = &pv, &av
		}
	}
	is.RevenuePrev = &[2]int64{prevTotal[0], sumTotal[0] / 3 / 100 * 100}
	is.ExpensePrev = &[2]int64{prevTotal[1], sumTotal[1] / 3 / 100 * 100}
	return nil
}
