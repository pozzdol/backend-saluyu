package model

import (
	"strings"

	"github.com/google/uuid"
)

// Contact is a person money is lent to or borrowed from, with their current position.
type Contact struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	LinkedUserID *uuid.UUID `json:"linked_user_id"`
	Receivable   int64      `json:"receivable"` // they owe me (1101)
	Payable      int64      `json:"payable"`    // I owe them (2101)
	Net          int64      `json:"net"`        // receivable - payable; positive = they owe me
}

type ContactRequest struct {
	Name string `json:"name" binding:"required,max=100"`
}

func (r *ContactRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
}

type ContactLine struct {
	EntryID     uuid.UUID `json:"entry_id"`
	EntryNo     int64     `json:"entry_no"`
	Date        string    `json:"entry_date"`
	Kind        string    `json:"kind"`
	Description string    `json:"description"`
	Code        string    `json:"account_code"`
	Debit       int64     `json:"debit"`
	Credit      int64     `json:"credit"`
	RunningNet  int64     `json:"running_net"`
}

type ContactDetail struct {
	Contact Contact       `json:"contact"`
	History []ContactLine `json:"history"`
}

// LoanRequest covers money between me and a contact:
//   - talangan: I pay for them (split bill: my_share of it is my own expense)
//   - pinjam: I borrow from them
//   - lunas_masuk: they pay me back
//   - lunas_keluar: I pay them back
type LoanRequest struct {
	Type             string     `json:"type" binding:"required,oneof=talangan pinjam lunas_masuk lunas_keluar"`
	ContactID        uuid.UUID  `json:"contact_id" binding:"required"`
	AccountID        uuid.UUID  `json:"account_id" binding:"required"` // the cash/bank account money moves through
	Amount           int64      `json:"amount" binding:"gt=0"`
	EntryDate        string     `json:"entry_date" binding:"required,datetime=2006-01-02"`
	Description      string     `json:"description" binding:"max=200"`
	MyShare          int64      `json:"my_share" binding:"gte=0,ltfield=Amount"`
	ExpenseAccountID *uuid.UUID `json:"expense_account_id"`
	// ConfirmOverpay turns a repayment larger than the debt into a debt the other way.
	ConfirmOverpay bool `json:"confirm_overpay"`
}

func (r *LoanRequest) Normalize() {
	r.Description = strings.TrimSpace(r.Description)
}
