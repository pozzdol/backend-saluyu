package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type Member struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
	Image string    `json:"img_url"`
}

type PendingInvite struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// Group is what an active member sees: other active members and invites not yet answered.
type Group struct {
	ID             uuid.UUID       `json:"id"`
	Name           string          `json:"name"`
	Members        []Member        `json:"members"`
	PendingInvites []PendingInvite `json:"pending_invites"`
}

type GroupRequest struct {
	Name string `json:"name" binding:"required,max=100"`
}

func (r *GroupRequest) Normalize() { r.Name = strings.TrimSpace(r.Name) }

type InviteRequest struct {
	Email string `json:"email" binding:"required,email,max=100"`
}

func (r *InviteRequest) Normalize() { r.Email = NormalizeEmail(r.Email) }

// MemberSummary is all a group member may see of another member's books:
// totals for one payday period, never a single transaction.
type MemberSummary struct {
	Member      Member     `json:"member"`
	Period      PeriodJSON `json:"period"`
	Income      int64      `json:"income"`
	Expense     int64      `json:"expense"`
	Surplus     int64      `json:"surplus"`
	SavingsRate float64    `json:"savings_rate"`
	ToSavings   int64      `json:"to_savings"`
	Budget      int64      `json:"budget"`        // sum of expense budgets
	BudgetSpent int64      `json:"budget_actual"` // spent on budgeted accounts
	Assets      int64      `json:"assets"`        // sum of asset balances at period end
}

type LinkContactRequest struct {
	// UserID links the contact to a member of a shared group; null unlinks it.
	UserID *uuid.UUID `json:"user_id"`
}

type InboxInvite struct {
	ID            uuid.UUID `json:"id"`
	GroupID       uuid.UUID `json:"group_id"`
	GroupName     string    `json:"group_name"`
	InvitedByName string    `json:"invited_by_name"`
	CreatedAt     time.Time `json:"created_at"`
}

// InboxLoan is a loan another member recorded with me, waiting for my approval.
// Delta is the change in what I owe them: positive = I owe more, negative = they owe me more.
type InboxLoan struct {
	ID        uuid.UUID `json:"id"`
	FromName  string    `json:"from_name"`
	Kind      string    `json:"kind"`
	Amount    int64     `json:"amount"`
	Delta     int64     `json:"delta"`
	Date      string    `json:"entry_date"`
	CreatedAt time.Time `json:"created_at"`
}

type Inbox struct {
	Invites []InboxInvite `json:"invites"`
	Loans   []InboxLoan   `json:"loans"`
	Changes []InboxChange `json:"changes"` // edits to loans already mirrored in my book
}

type ApproveLoanRequest struct {
	// AccountID is the approver's own account for the other side: the bank the money landed in,
	// or the expense it paid for (a dinner they paid on my behalf).
	AccountID uuid.UUID `json:"account_id" binding:"required"`
}

// LoanSource is the original entry behind a loan request, as the approver's side needs it.
type LoanSource struct {
	Status     string
	ToEntryID  *uuid.UUID
	FromUserID uuid.UUID
	FromName   string
	FromEmail  string
	Kind       string
	Date       time.Time
	Lines      []Line // only the lines on the contact linked to the approver
}
