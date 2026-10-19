package model

import (
	"time"

	"github.com/google/uuid"
)

// PeriodJSON is a payday period with dates as YYYY-MM-DD. Next and Prev are dates inside
// the neighbouring periods, ready to send back as ?date= for navigation.
type PeriodJSON struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Next  string `json:"next"`
	Prev  string `json:"prev"`
	Label string `json:"label"`
}

// AmountRow is one account's amount on its normal side, in sen.
type AmountRow struct {
	AccountID uuid.UUID `json:"account_id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Amount    int64     `json:"amount"`
	// Prev and Avg3 are filled when the income statement is compared: the previous period,
	// and the average of the three periods before this one.
	Prev *int64 `json:"prev,omitempty"`
	Avg3 *int64 `json:"avg3,omitempty"`
}

type BudgetRow struct {
	AccountID uuid.UUID `json:"account_id"`
	Name      string    `json:"name"`
	Budget    int64     `json:"budget"`
	Actual    int64     `json:"actual"`
	Remaining int64     `json:"remaining"`
	Over      bool      `json:"over"`
	Inherited bool      `json:"inherited"` // budget was set in an earlier period and carries forward
	// With the envelope on, Carry is the accumulated (budget - spent) of earlier periods and
	// Available = Budget + Carry; otherwise Carry is 0 and Available = Budget. Remaining and
	// Over are measured against Available.
	BudgetCarry bool    `json:"budget_carry"`
	Carry       int64   `json:"carry"`
	Available   int64   `json:"available"`
	NeedGroup   *string `json:"need_group"`
}

// GroupShare is one 50/30/20 group: what the period spent on it and its share of income.
type GroupShare struct {
	Group  string  `json:"group"` // needs, wants or savings
	Amount int64   `json:"amount"`
	Share  float64 `json:"share"`  // Amount / income, 0 without income
	Target float64 `json:"target"` // 0.5, 0.3 or 0.2
}

type BalanceRow struct {
	AccountID uuid.UUID `json:"account_id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Category  string    `json:"category"`
	Balance   int64     `json:"balance"`
}

type Dashboard struct {
	Period         PeriodJSON   `json:"period"`
	Income         int64        `json:"income"`
	Expense        int64        `json:"expense"`
	Surplus        int64        `json:"surplus"`
	SavingsRate    float64      `json:"savings_rate"`
	ToSavings      []AmountRow  `json:"to_savings"`
	ToSavingsTotal int64        `json:"to_savings_total"`
	Budgets        []BudgetRow  `json:"budgets"`
	Balances       []BalanceRow `json:"balances"`
}

type BudgetList struct {
	Period PeriodJSON   `json:"period"`
	Rows   []BudgetRow  `json:"rows"`
	Income int64        `json:"income"`
	Groups []GroupShare `json:"groups"`
}

// BudgetRequest sets the budget of the period containing Date; it holds until changed.
type BudgetRequest struct {
	Date   string `json:"date" binding:"required,datetime=2006-01-02"`
	Amount int64  `json:"amount" binding:"gte=0"`
}

type IncomeStatement struct {
	Period       PeriodJSON  `json:"period"`
	Revenue      []AmountRow `json:"revenue"`
	RevenueTotal int64       `json:"revenue_total"`
	Expense      []AmountRow `json:"expense"`
	ExpenseTotal int64       `json:"expense_total"`
	NetIncome    int64       `json:"net_income"`
	// Compared totals: [previous period, average of the three before].
	RevenuePrev *[2]int64 `json:"revenue_compare,omitempty"`
	ExpensePrev *[2]int64 `json:"expense_compare,omitempty"`
}

type BalanceSheet struct {
	AsOf             string      `json:"as_of"`
	Assets           []AmountRow `json:"assets"`
	AssetsTotal      int64       `json:"assets_total"`
	Liabilities      []AmountRow `json:"liabilities"`
	LiabilitiesTotal int64       `json:"liabilities_total"`
	Equity           []AmountRow `json:"equity"`
	EquityTotal      int64       `json:"equity_total"`
	// CurrentEarnings is revenue minus expense never closed into equity; it keeps the sheet balanced.
	CurrentEarnings int64 `json:"current_earnings"`
	Balanced        bool  `json:"balanced"`
}

// Movement is the debit and credit total of one account for one entry kind in a date range.
type Movement struct {
	AccountID                  uuid.UUID
	Code, Name, Type, Category string
	Kind                       string
	Debit, Credit              int64
}

type Budget struct {
	AccountID   uuid.UUID
	Name        string
	PeriodStart time.Time
	Amount      int64
}

// ExportTable is one CSV file of the data download: a header and text cells.
type ExportTable struct {
	Name    string
	Columns []string
	Rows    [][]string
}

// EntryLine is one journal line with its entry and account, for reports that look inside
// entries (cash flow, daily totals).
type EntryLine struct {
	EntryID                    uuid.UUID
	EntryNo                    int64
	Description                string
	Kind                       string
	Date                       time.Time
	AccountID                  uuid.UUID
	Code, Name, Type, Category string
	Debit, Credit              int64
}

type DayTotal struct {
	Date    string `json:"date"`
	Income  int64  `json:"income"`
	Expense int64  `json:"expense"`
}

// SafePerDay is how much can still be spent each day until the next payday.
type SafePerDay struct {
	Amount    int64  `json:"amount"`
	Remaining int64  `json:"remaining"`
	DaysLeft  int    `json:"days_left"`
	Basis     string `json:"basis"` // budget (unspent budgets) or surplus (income minus spending)
}

// Insight is one observation; the frontend words it in the user's language.
type Insight struct {
	Kind      string     `json:"kind"` // category_up, category_down, savings_up, savings_down
	AccountID *uuid.UUID `json:"account_id,omitempty"`
	Name      string     `json:"name,omitempty"`
	Amount    int64      `json:"amount"`  // the difference in sen, always positive
	Percent   float64    `json:"percent"` // the change relative to the comparison
}

type BudgetAlert struct {
	AccountID uuid.UUID `json:"account_id"`
	Name      string    `json:"name"`
	Budget    int64     `json:"budget"`
	Actual    int64     `json:"actual"`
	Level     string    `json:"level"` // warn (80% or more) or over (more than 100%)
}

type Insights struct {
	Period       PeriodJSON    `json:"period"`
	SafePerDay   SafePerDay    `json:"safe_per_day"`
	ByCategory   []AmountRow   `json:"by_category"`
	ExpenseTotal int64         `json:"expense_total"`
	Daily        []DayTotal    `json:"daily"`
	Insights     []Insight     `json:"insights"`
	BudgetAlerts []BudgetAlert `json:"budget_alerts"`
}

type TrendPoint struct {
	Period     PeriodJSON  `json:"period"`
	Income     int64       `json:"income"`
	Expense    int64       `json:"expense"`
	NetWorth   int64       `json:"net_worth"` // assets minus liabilities at the period's end
	ByCategory []AmountRow `json:"by_category"`
}

// Ratio is one health figure with the thresholds it is judged by.
type Ratio struct {
	Value     float64 `json:"value"`
	Good      float64 `json:"good"`        // at or past this is good
	Warn      float64 `json:"warn"`        // between warn and good is a warning
	Status    string  `json:"status"`      // good, warn, bad, or none without enough data
	LowerIsOK bool    `json:"lower_is_ok"` // true when smaller values are better (debt ratio)
}

type Health struct {
	EmergencyMonths Ratio `json:"emergency_months"`
	DebtRatio       Ratio `json:"debt_ratio"`
	Savings3        Ratio `json:"savings_3"`
	Savings6        Ratio `json:"savings_6"`
	Savings12       Ratio `json:"savings_12"`
	Cash            int64 `json:"cash"`
	AvgExpense      int64 `json:"avg_expense"`
	AvgIncome       int64 `json:"avg_income"`
	AvgDebtPayments int64 `json:"avg_debt_payments"`
	Periods         int   `json:"periods"` // complete periods with activity that the averages use
}

type CashFlowSection struct {
	Rows  []AmountRow `json:"rows"` // positive = cash in
	Total int64       `json:"total"`
}

type CashFlow struct {
	Period    PeriodJSON      `json:"period"`
	Beginning int64           `json:"beginning"`
	Operating CashFlowSection `json:"operating"`
	Investing CashFlowSection `json:"investing"`
	Financing CashFlowSection `json:"financing"`
	Opening   int64           `json:"opening"` // starting balances entered inside the period
	Net       int64           `json:"net"`
	Ending    int64           `json:"ending"`
}

type TaxSummary struct {
	Year             int         `json:"year"`
	AsOf             string      `json:"as_of"`
	Assets           []AmountRow `json:"assets"`
	AssetsTotal      int64       `json:"assets_total"`
	Liabilities      []AmountRow `json:"liabilities"`
	LiabilitiesTotal int64       `json:"liabilities_total"`
	Income           []AmountRow `json:"income"`
	IncomeTotal      int64       `json:"income_total"`
}
