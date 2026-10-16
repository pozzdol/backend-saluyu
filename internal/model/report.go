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
}

type BudgetRow struct {
	AccountID uuid.UUID `json:"account_id"`
	Name      string    `json:"name"`
	Budget    int64     `json:"budget"`
	Actual    int64     `json:"actual"`
	Remaining int64     `json:"remaining"`
	Over      bool      `json:"over"`
	Inherited bool      `json:"inherited"` // budget was set in an earlier period and carries forward
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
	Period PeriodJSON  `json:"period"`
	Rows   []BudgetRow `json:"rows"`
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
