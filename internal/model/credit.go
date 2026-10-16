package model

import (
	"strings"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/installment"
)

// Card is a credit card or paylater account with its billing cycle.
type Card struct {
	Account       Account `json:"account"`
	StatementDay  int     `json:"statement_day"`
	DueDay        int     `json:"due_day"`
	CreditLimit   int64   `json:"credit_limit"` // sen, 0 = not set
	MinPaymentPct int     `json:"min_payment_pct"`
}

type CardSettings struct {
	StatementDay  int   `json:"statement_day" binding:"min=1,max=28"`
	DueDay        int   `json:"due_day" binding:"min=1,max=28"`
	CreditLimit   int64 `json:"credit_limit" binding:"gte=0"`
	MinPaymentPct int   `json:"min_payment_pct" binding:"min=0,max=100"`
}

type CardRequest struct {
	Name string `json:"name" binding:"required,max=100"`
	Kind string `json:"kind" binding:"required,oneof=credit_card paylater"`
	CardSettings
}

func (r *CardRequest) Normalize() { r.Name = strings.TrimSpace(r.Name) }

// Statement is one closed billing cycle (start, end] and how much of it is still owed.
type Statement struct {
	Start        string      `json:"start"` // day after the previous statement date
	End          string      `json:"end"`   // the statement date
	DueDate      string      `json:"due_date"`
	Opening      int64       `json:"opening"`  // owed at the previous statement date
	Charges      int64       `json:"charges"`  // purchases and fees in the cycle
	Payments     int64       `json:"payments"` // payments and refunds in the cycle
	Total        int64       `json:"total"`    // owed at the statement date: the bill
	Minimum      int64       `json:"minimum"`
	PaidSince    int64       `json:"paid_since"` // paid after the statement date
	Remaining    int64       `json:"remaining"`
	MinRemaining int64       `json:"min_remaining"`
	Status       string      `json:"status"` // paid, due, min_paid, overdue
	Lines        []LedgerRow `json:"lines,omitempty"`
}

// Cycle is the open cycle: everything charged since the last statement date.
type Cycle struct {
	Start    string      `json:"start"`
	End      string      `json:"end"` // the next statement date
	Charges  int64       `json:"charges"`
	Payments int64       `json:"payments"`
	Lines    []LedgerRow `json:"lines,omitempty"`
}

type CardSummary struct {
	Card
	Owed      int64     `json:"owed"`      // everything owed today
	Available *int64    `json:"available"` // limit minus owed; null without a limit
	Last      Statement `json:"last"`
	Current   Cycle     `json:"current"`
}

// Plan is a loan repaid in scheduled installments.
type Plan struct {
	ID                uuid.UUID          `json:"id"`
	Account           Account            `json:"account"` // the loan (liability)
	InterestAccountID uuid.UUID          `json:"interest_account_id"`
	Principal         int64              `json:"principal"`
	AnnualRateBP      int                `json:"annual_rate_bp"`
	TenorMonths       int                `json:"tenor_months"`
	Method            installment.Method `json:"method"`
	FirstDueDate      string             `json:"first_due_date"`
	Paid              []PaidInstallment  `json:"-"`
}

type PaidInstallment struct {
	Seq     int       `json:"seq"`
	EntryID uuid.UUID `json:"entry_id"`
	EntryNo int64     `json:"entry_no"`
}

type PlanRow struct {
	installment.Row
	Paid *PaidInstallment `json:"paid"`
}

type PlanSummary struct {
	Plan
	PaidCount     int              `json:"paid_count"`
	Remaining     int64            `json:"remaining"` // principal left after the paid rows
	TotalInterest int64            `json:"total_interest"`
	Next          *installment.Row `json:"next"` // nil once fully paid
	Rows          []PlanRow        `json:"rows,omitempty"`
}

type PlanRequest struct {
	Name              string     `json:"name" binding:"required,max=100"`
	Principal         int64      `json:"principal" binding:"gt=0"`
	AnnualRate        float64    `json:"annual_rate" binding:"gte=0,lte=100"` // percent a year, e.g. 10.5
	TenorMonths       int        `json:"tenor_months" binding:"min=1,max=360"`
	Method            string     `json:"method" binding:"required,oneof=flat annuity"`
	FirstDueDate      string     `json:"first_due_date" binding:"required,datetime=2006-01-02"`
	InterestAccountID *uuid.UUID `json:"interest_account_id"` // default 5108 Bunga & Denda
	// ReceivedIntoID records the loan money arriving (an asset) or what it bought (an expense).
	// Leave it out for a loan that was already running before the books opened.
	ReceivedIntoID *uuid.UUID `json:"received_into_id"`
	EntryDate      string     `json:"entry_date" binding:"omitempty,datetime=2006-01-02"`
}

func (r *PlanRequest) Normalize() { r.Name = strings.TrimSpace(r.Name) }

type PayInstallmentRequest struct {
	FromAccountID uuid.UUID `json:"from_account_id" binding:"required"`
	EntryDate     string    `json:"entry_date" binding:"required,datetime=2006-01-02"`
}
