package model

import (
	"strings"

	"github.com/google/uuid"
)

type RecurringRule struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Kind          string    `json:"kind"`
	FromAccountID uuid.UUID `json:"from_account_id"`
	ToAccountID   uuid.UUID `json:"to_account_id"`
	Amount        int64     `json:"amount"`
	Frequency     string    `json:"frequency"`
	Day           *int      `json:"day"`
	Mode          string    `json:"mode"`
	NextDate      string    `json:"next_date"`
	EndDate       *string   `json:"end_date"`
	LastError     *string   `json:"last_error"`
	UserID        uuid.UUID `json:"-"`
}

type RecurringRequest struct {
	Name          string    `json:"name" binding:"required,max=100"`
	Kind          string    `json:"kind" binding:"required,oneof=income expense transfer"`
	FromAccountID uuid.UUID `json:"from_account_id" binding:"required"`
	ToAccountID   uuid.UUID `json:"to_account_id" binding:"required"`
	Amount        int64     `json:"amount" binding:"gt=0"`
	Frequency     string    `json:"frequency" binding:"required,oneof=daily weekly monthly payday"`
	Day           *int      `json:"day" binding:"omitnil,min=0,max=31"`
	Mode          string    `json:"mode" binding:"required,oneof=remind auto"`
	StartDate     string    `json:"start_date" binding:"required,datetime=2006-01-02"`
	EndDate       *string   `json:"end_date" binding:"omitnil,datetime=2006-01-02"`
}

func (r *RecurringRequest) Normalize() { r.Name = strings.TrimSpace(r.Name) }

// DueOccurrence is one date a rule falls on that is not posted or skipped yet.
type DueOccurrence struct {
	Rule    RecurringRule `json:"rule"`
	DueDate string        `json:"due_date"`
}

type ConfirmRequest struct {
	DueDate   string `json:"due_date" binding:"required,datetime=2006-01-02"`
	Amount    int64  `json:"amount" binding:"gte=0"` // 0 = the rule's amount
	EntryDate string `json:"entry_date" binding:"omitempty,datetime=2006-01-02"`
}

type SkipRequest struct {
	DueDate string `json:"due_date" binding:"required,datetime=2006-01-02"`
}
