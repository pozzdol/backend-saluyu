package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// EntrySnapshot is an entry as stored in a revision or a change proposal.
type EntrySnapshot struct {
	Date        string  `json:"entry_date"`
	PeriodDate  *string `json:"period_date"`
	Description string  `json:"description"`
	Lines       []Line  `json:"lines"`
}

func Snapshot(e *Entry) EntrySnapshot {
	return EntrySnapshot{Date: e.EntryDate.Format(time.DateOnly), PeriodDate: DateString(e.PeriodDate), Description: e.Description, Lines: e.Lines}
}

type Revision struct {
	ChangedAt time.Time     `json:"changed_at"`
	ChangedBy string        `json:"changed_by"` // name of who made it (the other member, for an approved change)
	Before    EntrySnapshot `json:"before"`
	After     EntrySnapshot `json:"after"`
}

// EntryDetail is one entry with its history and its link to another member's book.
type EntryDetail struct {
	Entry         Entry      `json:"entry"`
	Revisions     []Revision `json:"revisions"`
	LoanStatus    string     `json:"loan_status"` // "", pending, approved, rejected
	Mirrored      bool       `json:"mirrored"`    // this entry is the mirror of someone else's loan
	ChangePending bool       `json:"change_pending"`
}

type LineInput struct {
	AccountID uuid.UUID  `json:"account_id" binding:"required"`
	ContactID *uuid.UUID `json:"contact_id"`
	Debit     int64      `json:"debit" binding:"gte=0"`
	Credit    int64      `json:"credit" binding:"gte=0"`
}

// LinesRequest edits an entry line by line: cash counts, reconciliation adjustments,
// or any correction the simple forms cannot express.
type LinesRequest struct {
	EntryDate   string      `json:"entry_date" binding:"required,datetime=2006-01-02"`
	Description string      `json:"description" binding:"max=200"`
	Lines       []LineInput `json:"lines" binding:"required,min=2,dive"`
}

func (r *LinesRequest) Normalize() { r.Description = strings.TrimSpace(r.Description) }

// ChangeProposal is the sender's edited loan, waiting for the other member's approval.
// MirrorLines are its lines on the contact linked to the approver, with account codes,
// which is all the approver's side needs to rebuild the mirror.
type ChangeProposal struct {
	Entry       EntrySnapshot `json:"entry"`
	MirrorLines []Line        `json:"mirror_lines"`
}

type AmountDate struct {
	Amount int64  `json:"amount"`
	Date   string `json:"entry_date"`
}

// InboxChange is an edit another member made to a loan already in my book.
type InboxChange struct {
	ID        uuid.UUID  `json:"id"`
	FromName  string     `json:"from_name"`
	Before    AmountDate `json:"before"`
	After     AmountDate `json:"after"`
	CreatedAt time.Time  `json:"created_at"`
}

// ChangeSource is what approving a change needs to read first.
type ChangeSource struct {
	Status        string
	LoanRequestID uuid.UUID
	FromUserID    uuid.UUID
	FromName      string
	FromEntryID   uuid.UUID
	ToEntryID     uuid.UUID
	Proposal      ChangeProposal
}
