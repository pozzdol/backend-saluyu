package model

import (
	"strings"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/statement"
)

type CategoryRule struct {
	ID        uuid.UUID `json:"id"`
	Keyword   string    `json:"keyword"`
	MinAmount *int64    `json:"min_amount"`
	MaxAmount *int64    `json:"max_amount"`
	AccountID uuid.UUID `json:"account_id"`
}

type CategoryRuleRequest struct {
	Keyword   string    `json:"keyword" binding:"required,max=100"`
	MinAmount *int64    `json:"min_amount" binding:"omitnil,gte=0"`
	MaxAmount *int64    `json:"max_amount" binding:"omitnil,gte=0"`
	AccountID uuid.UUID `json:"account_id" binding:"required"`
}

func (r *CategoryRuleRequest) Normalize() { r.Keyword = strings.TrimSpace(r.Keyword) }

// Matches says whether a rule applies to a description and amount (sen, sign ignored).
func (r CategoryRule) Matches(description string, amount int64) bool {
	if amount < 0 {
		amount = -amount
	}
	return strings.Contains(strings.ToLower(description), strings.ToLower(r.Keyword)) &&
		(r.MinAmount == nil || amount >= *r.MinAmount) && (r.MaxAmount == nil || amount <= *r.MaxAmount)
}

// ImportRow is one statement row in the preview.
type ImportRow struct {
	statement.Row
	Hash               string     `json:"hash"`   // hex SHA-256: date, amount, description, nth copy
	Status             string     `json:"status"` // new, duplicate (imported before), matched (already recorded by hand)
	EntryID            *uuid.UUID `json:"entry_id"`
	EntryNo            *int64     `json:"entry_no"`
	SuggestedAccountID *uuid.UUID `json:"suggested_account_id"`
}

type ImportPreview struct {
	Mapping statement.Mapping     `json:"mapping"`
	Headers []string              `json:"headers"` // the header row's cells, to name columns in the mapping form
	Rows    []ImportRow           `json:"rows"`
	Errors  []statement.LineError `json:"errors"`
}

type ImportCommitRow struct {
	Hash         string     `json:"hash" binding:"required,len=64,hexadecimal"`
	Date         string     `json:"date" binding:"required,datetime=2006-01-02"`
	Description  string     `json:"description" binding:"max=500"`
	Amount       int64      `json:"amount" binding:"required"`
	AccountID    *uuid.UUID `json:"account_id"`     // the category; default 5199 (out) or 4199 (in)
	MatchEntryID *uuid.UUID `json:"match_entry_id"` // link this row to an entry recorded by hand
}

type ImportCommitRequest struct {
	AccountID uuid.UUID             `json:"account_id" binding:"required"`
	Mapping   statement.Mapping     `json:"mapping"`
	Rows      []ImportCommitRow     `json:"rows" binding:"required,max=2000,dive"`
	Remember  []CategoryRuleRequest `json:"remember" binding:"max=50,dive"`
}

type ImportResult struct {
	Posted  int `json:"posted"`
	Matched int `json:"matched"`
	Skipped int `json:"skipped"` // already imported
}

// ImportedRow is what the repository stores per row: a new entry or a link to an old one.
type ImportedRow struct {
	Hash    []byte
	Entry   *Entry     // new entry to post, or
	EntryID *uuid.UUID // an existing one to link
}

// MatchCandidate is an entry on the account that no statement row is linked to yet.
type MatchCandidate struct {
	EntryID uuid.UUID
	EntryNo int64
	Date    string
	Amount  int64 // debit minus credit on the account
}
