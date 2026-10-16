package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// All money is int64 sen (1/100 rupiah). Float would turn 10,826,012.92 into 10826012.919999...
type Account struct {
	ID            uuid.UUID `json:"id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`
	Category      string    `json:"category"`
	NormalBalance string    `json:"normal_balance"`
	IsActive      bool      `json:"is_active"`
}

// NormalBalance is the side that increases an account: debit for Asset and Expense, credit for the rest.
func NormalBalance(accountType string) string {
	if accountType == "Asset" || accountType == "Expense" {
		return "Debit"
	}
	return "Credit"
}

type TemplateAccount struct{ Code, Name, Type, Category string }

// DefaultAccounts is the "Pribadi" chart of accounts every new user starts with.
// Keep in sync with migration 000004, which seeded the users that existed before it.
var DefaultAccounts = []TemplateAccount{
	{"1001", "Bank", "Asset", "Kas & Bank"},
	{"1002", "Cash", "Asset", "Kas & Bank"},
	{"1003", "Tabungan", "Asset", "Tabungan"},
	{"1004", "E-wallet", "Asset", "Kas & Bank"},
	{"1005", "Investasi", "Asset", "Investasi"},
	{"1101", "Piutang", "Asset", "Piutang"},
	{"2101", "Utang", "Liability", "Utang"},
	{"3001", "Ekuitas Saldo Awal", "Equity", "Ekuitas"},
	{"4001", "Gaji", "Revenue", "Pendapatan"},
	{"4002", "Bunga", "Revenue", "Pendapatan"},
	{"4003", "Pengembalian", "Revenue", "Pendapatan Lain"},
	{"4199", "Pendapatan Lain-lain", "Revenue", "Pendapatan Lain"},
	{"5101", "Transportasi", "Expense", "Beban"},
	{"5102", "Makan", "Expense", "Beban"},
	{"5103", "Jajan", "Expense", "Beban"},
	{"5104", "Entertainment", "Expense", "Beban"},
	{"5105", "Keluarga", "Expense", "Beban"},
	{"5107", "Admin", "Expense", "Beban"},
	{"5199", "Beban Tunai Lain-lain", "Expense", "Beban"},
}

type CreateAccountRequest struct {
	Code     string `json:"code" binding:"required,numeric,max=20"`
	Name     string `json:"name" binding:"required,max=100"`
	Type     string `json:"type" binding:"required,oneof=Asset Liability Equity Revenue Expense"`
	Category string `json:"category" binding:"max=100"`
}

func (r *CreateAccountRequest) Normalize() {
	r.Code = strings.TrimSpace(r.Code)
	r.Name = strings.TrimSpace(r.Name) // the Excel had "Transportasi " with a trailing space
	r.Category = strings.TrimSpace(r.Category)
}

// UpdateAccountRequest is partial: a field left out stays unchanged. Type and code never change,
// because existing journal lines were posted under them.
type UpdateAccountRequest struct {
	Name     *string `json:"name" binding:"omitnil,min=1,max=100"`
	Category *string `json:"category" binding:"omitnil,max=100"`
	IsActive *bool   `json:"is_active"`
}

func (r *UpdateAccountRequest) Normalize() {
	for _, p := range []*string{r.Name, r.Category} {
		if p != nil {
			*p = strings.TrimSpace(*p)
		}
	}
}

type Line struct {
	AccountID uuid.UUID `json:"account_id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Debit     int64     `json:"debit"`
	Credit    int64     `json:"credit"`
}

type Entry struct {
	ID          uuid.UUID `json:"id"`
	EntryNo     int64     `json:"entry_no"`
	EntryDate   time.Time `json:"-"`
	Date        string    `json:"entry_date"` // EntryDate as YYYY-MM-DD: a calendar date, not an instant
	Kind        string    `json:"kind"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	Lines       []Line    `json:"lines"`
}

// PostEntryRequest is the simple form: money moves from one account to another.
// income: Revenue -> Asset, expense: Asset -> Expense, transfer: Asset -> Asset.
type PostEntryRequest struct {
	Kind          string    `json:"kind" binding:"required,oneof=income expense transfer"`
	EntryDate     string    `json:"entry_date" binding:"required,datetime=2006-01-02"`
	Amount        int64     `json:"amount" binding:"gt=0"`
	FromAccountID uuid.UUID `json:"from_account_id" binding:"required"`
	ToAccountID   uuid.UUID `json:"to_account_id" binding:"required"`
	Description   string    `json:"description" binding:"max=200"`
}

func (r *PostEntryRequest) Normalize() {
	r.Description = strings.TrimSpace(r.Description)
}

type EntryFilter struct {
	From, To  *time.Time
	Kind      *string
	AccountID *uuid.UUID
	Query     string
	Limit     int
	Offset    int
}

type LedgerRow struct {
	EntryID     uuid.UUID `json:"entry_id"`
	EntryNo     int64     `json:"entry_no"`
	Date        string    `json:"entry_date"`
	Kind        string    `json:"kind"`
	Description string    `json:"description"`
	Counter     string    `json:"counter"` // the other account(s) of the entry
	Debit       int64     `json:"debit"`
	Credit      int64     `json:"credit"`
	Balance     int64     `json:"balance"`
}

type Ledger struct {
	Account Account     `json:"account"`
	Opening int64       `json:"opening"`
	Rows    []LedgerRow `json:"rows"`
	Closing int64       `json:"closing"`
}

type TrialBalanceRow struct {
	AccountID     uuid.UUID `json:"account_id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`
	Debit         int64     `json:"debit"`  // total debits posted
	Credit        int64     `json:"credit"` // total credits posted
	DebitBalance  int64     `json:"debit_balance"`
	CreditBalance int64     `json:"credit_balance"`
}

type TrialBalance struct {
	AsOf        string            `json:"as_of"`
	Rows        []TrialBalanceRow `json:"rows"`
	TotalDebit  int64             `json:"total_debit"`
	TotalCredit int64             `json:"total_credit"`
	Balanced    bool              `json:"balanced"`
}
