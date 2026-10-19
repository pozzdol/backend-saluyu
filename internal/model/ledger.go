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
	Icon          string    `json:"icon"`     // lucide icon name, "" = default for the type
	Color         string    `json:"color"`    // palette token, "" = default for the type
	Uses30d       int       `json:"uses_30d"` // journal lines in the last 30 days, for "most used first"
	// HasEntries: the account has at least one journal line. Quick Add hides system
	// categories (5108, 5199) until they do.
	HasEntries bool `json:"has_entries"`
	// Kind is "general", or for liabilities "credit_card", "paylater" (a card with a bill)
	// or "loan" (an installment plan).
	Kind string `json:"kind"`
	// BudgetCarry rolls the unspent budget into the next period (envelope); BudgetCarrySince
	// is the day it was switched on. NeedGroup is the 50/30/20 group: needs, wants, savings or nil.
	BudgetCarry      bool       `json:"budget_carry"`
	BudgetCarrySince *time.Time `json:"-"`
	NeedGroup        *string    `json:"need_group"`
}

// IsCard reports whether purchases may be charged to this account.
func (a Account) IsCard() bool { return a.Kind == "credit_card" || a.Kind == "paylater" }

// FixedAssetCategory holds vehicles, property and gadgets: owned, but never spendable.
const FixedAssetCategory = "Aset Tetap"

// IsFixedAsset reports whether the account is a fixed asset (never counted as liquid money).
func (a Account) IsFixedAsset() bool { return a.Type == "Asset" && a.Category == FixedAssetCategory }

// NormalBalance is the side that increases an account: debit for Asset and Expense, credit for the rest.
func NormalBalance(accountType string) string {
	if accountType == "Asset" || accountType == "Expense" {
		return "Debit"
	}
	return "Credit"
}

type TemplateAccount struct{ Code, Name, Type, Category, Icon, Color string }

// DefaultAccounts is the "Pribadi" chart of accounts every new user starts with.
// Keep in sync with migration 000004, which seeded the users that existed before it.
// Users seeded before Fase 12 may still have the English names Cash, E-wallet and Admin;
// the app offers to rename them (the frontend lists the old names).
var DefaultAccounts = []TemplateAccount{
	{"1001", "Bank", "Asset", "Kas & Bank", "landmark", "blue"},
	{"1002", "Tunai", "Asset", "Kas & Bank", "wallet", "green"},
	{"1003", "Tabungan", "Asset", "Tabungan", "piggy-bank", "teal"},
	{"1004", "Dompet Digital", "Asset", "Kas & Bank", "smartphone", "violet"},
	{"1005", "Investasi", "Asset", "Investasi", "trending-up", "indigo"},
	{"1101", "Piutang", "Asset", "Piutang", "hand-coins", "orange"},
	{"1301", "Kendaraan", "Asset", "Aset Tetap", "car", "sky"},
	{"1302", "Properti", "Asset", "Aset Tetap", "house", "emerald"},
	{"1303", "Elektronik", "Asset", "Aset Tetap", "tv", "slate"},
	{"2101", "Utang", "Liability", "Utang", "handshake", "amber"},
	{"3001", "Ekuitas Saldo Awal", "Equity", "Ekuitas", "scale", "slate"},
	{"3002", "Penyesuaian Nilai Aset", "Equity", "Ekuitas", "scale", "violet"},
	{"4001", "Gaji", "Revenue", "Pendapatan", "briefcase", "green"},
	{"4002", "Bunga", "Revenue", "Pendapatan", "percent", "teal"},
	{"4003", "Pengembalian", "Revenue", "Pendapatan Lain", "rotate-ccw", "sky"},
	{"4199", "Pendapatan Lain-lain", "Revenue", "Pendapatan Lain", "sparkles", "yellow"},
	{"5101", "Transportasi", "Expense", "Beban", "car", "sky"},
	{"5102", "Makan", "Expense", "Beban", "utensils", "orange"},
	{"5103", "Jajan", "Expense", "Beban", "cookie", "pink"},
	{"5104", "Hiburan", "Expense", "Beban", "clapperboard", "violet"},
	{"5105", "Keluarga", "Expense", "Beban", "house", "rose"},
	{"5107", "Biaya Admin Bank", "Expense", "Beban", "receipt", "slate"},
	{"5108", "Denda & Bunga Lain", "Expense", "Beban", "percent", "red"},
	{"5111", "Bunga Pinjaman", "Expense", "Beban", "percent", "amber"},
	{"5112", "Bunga & Denda Kartu", "Expense", "Beban", "credit-card", "red"},
	{"5199", "Beban Tunai Lain-lain", "Expense", "Beban", "coins", "zinc"},
}

// DefaultNeedGroup is the 50/30/20 group of the template accounts, by code.
var DefaultNeedGroup = map[string]string{
	"5101": "needs", "5102": "needs", "5105": "needs", "5107": "needs",
	"5103": "wants", "5104": "wants",
	"1003": "savings", "1005": "savings",
}

type CreateAccountRequest struct {
	Code     string `json:"code" binding:"required,numeric,max=20"`
	Name     string `json:"name" binding:"required,max=100"`
	Type     string `json:"type" binding:"required,oneof=Asset Liability Equity Revenue Expense"`
	Category string `json:"category" binding:"max=100"`
	Icon     string `json:"icon" binding:"omitempty,oneof=wallet landmark piggy-bank smartphone trending-up hand-coins handshake scale briefcase percent rotate-ccw sparkles car utensils cookie clapperboard house receipt coins shopping-cart shopping-bag coffee bus fuel plane heart-pulse graduation-cap baby gift shirt zap wifi phone dumbbell gamepad-2 book paw-print wrench tv credit-card banknote building-2 store"`
	Color    string `json:"color" binding:"omitempty,oneof=slate zinc red orange amber yellow lime green emerald teal cyan sky blue indigo violet purple fuchsia pink rose"`
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
	Icon     *string `json:"icon" binding:"omitnil,oneof=wallet landmark piggy-bank smartphone trending-up hand-coins handshake scale briefcase percent rotate-ccw sparkles car utensils cookie clapperboard house receipt coins shopping-cart shopping-bag coffee bus fuel plane heart-pulse graduation-cap baby gift shirt zap wifi phone dumbbell gamepad-2 book paw-print wrench tv credit-card banknote building-2 store"`
	Color    *string `json:"color" binding:"omitnil,oneof=slate zinc red orange amber yellow lime green emerald teal cyan sky blue indigo violet purple fuchsia pink rose"`
	// BudgetCarry switches the envelope on or off. NeedGroup "none" clears the 50/30/20 group.
	BudgetCarry *bool   `json:"budget_carry"`
	NeedGroup   *string `json:"need_group" binding:"omitnil,oneof=needs wants savings none"`
}

func (r *UpdateAccountRequest) Normalize() {
	for _, p := range []*string{r.Name, r.Category} {
		if p != nil {
			*p = strings.TrimSpace(*p)
		}
	}
}

// RevalueRequest sets a fixed asset's value on a date; the difference is booked to 3002.
type RevalueRequest struct {
	Value     int64  `json:"value" binding:"gte=0"` // sen
	EntryDate string `json:"entry_date" binding:"required,datetime=2006-01-02"`
}

type Line struct {
	AccountID uuid.UUID `json:"account_id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Debit     int64     `json:"debit"`
	Credit    int64     `json:"credit"`
	// ContactID is set only on 1101 Piutang and 2101 Utang lines: who owes, or is owed.
	ContactID   *uuid.UUID `json:"contact_id"`
	ContactName string     `json:"contact_name,omitempty"`
}

type Entry struct {
	ID        uuid.UUID `json:"id"`
	EntryNo   int64     `json:"entry_no"`
	EntryDate time.Time `json:"-"`
	Date      string    `json:"entry_date"` // EntryDate as YYYY-MM-DD: a calendar date, not an instant
	// PeriodDate moves the entry into another period for period reports (salary paid early).
	// nil means EntryDate. The balance sheet always uses EntryDate: that is when the money moved.
	PeriodDate    *time.Time `json:"-"`
	PeriodDateStr *string    `json:"period_date"`
	Kind          string     `json:"kind"`
	Description   string     `json:"description"`
	CreatedAt     time.Time  `json:"created_at"`
	Lines         []Line     `json:"lines"`
	// ClientID is chosen by a client that may send the same entry twice (an offline queue).
	ClientID *uuid.UUID `json:"client_id,omitempty"`
	// BalanceAfter is the balance of the filtered account after this entry; set only when
	// the list is filtered to one account.
	BalanceAfter *int64 `json:"balance_after,omitempty"`
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
	// NextPeriod counts an income in the period after entry_date's (salary arriving before payday).
	NextPeriod bool `json:"next_period"`
	// Fee and FeeAccountID add a bank charge to a transfer, paid from the same source.
	Fee          int64      `json:"fee" binding:"gte=0"`
	FeeAccountID *uuid.UUID `json:"fee_account_id"`
	// ClientID makes retries safe: the same id posts one entry and returns it after that.
	ClientID *uuid.UUID `json:"client_id"`
}

// ReverseRequest dates a reversing entry (default: today).
type ReverseRequest struct {
	EntryDate string `json:"entry_date" binding:"omitempty,datetime=2006-01-02"`
}

func (r *PostEntryRequest) Normalize() {
	r.Description = strings.TrimSpace(r.Description)
}

type EntryFilter struct {
	From, To  *time.Time
	Kind      *string
	AccountID *uuid.UUID
	Query     string
	// AmountMin and AmountMax bound the entry amount in sen (both inclusive, either optional).
	AmountMin, AmountMax *int64
	Limit                int
	Offset               int
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

type OpeningItem struct {
	AccountID uuid.UUID `json:"account_id" binding:"required"`
	// ContactID is required for 1101 Piutang and 2101 Utang: one item per person.
	ContactID *uuid.UUID `json:"contact_id"`
	Amount    int64      `json:"amount" binding:"gte=0"`
}

// OpeningRequest sets the real balance of each asset and liability account on the day the books open.
type OpeningRequest struct {
	StartDate string        `json:"start_date" binding:"required,datetime=2006-01-02"`
	Items     []OpeningItem `json:"items" binding:"dive"`
}

type Opening struct {
	StartDate *string `json:"start_date"`
	Entry     *Entry  `json:"entry"`    // nil when not opened yet or every balance was zero
	Editable  bool    `json:"editable"` // false once any other transaction exists
}

// ReconcileRequest compares the books with reality: cash counted in the wallet, or a bank statement.
type ReconcileRequest struct {
	AccountID uuid.UUID `json:"account_id" binding:"required"`
	Date      string    `json:"date" binding:"required,datetime=2006-01-02"`
	Actual    int64     `json:"actual" binding:"gte=0"`
	// CounterAccountID absorbs the difference. Cash counts pick it themselves (5199 or 4199).
	CounterAccountID *uuid.UUID `json:"counter_account_id"`
}

type ReconcileResult struct {
	Book   int64  `json:"book"`
	Actual int64  `json:"actual"`
	Diff   int64  `json:"diff"` // actual - book; 0 posts nothing
	Entry  *Entry `json:"entry"`
}
