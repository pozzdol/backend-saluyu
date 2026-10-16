package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Goal is money set aside for something: its progress is the balance of the account holding it.
type Goal struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Target        int64     `json:"target"`
	Deadline      *string   `json:"deadline"`
	AccountID     uuid.UUID `json:"account_id"`
	Saved         int64     `json:"saved"`
	Remaining     int64     `json:"remaining"`
	MonthlyNeeded *int64    `json:"monthly_needed"` // null without a deadline
	MonthsLeft    *int      `json:"months_left"`
	UserID        uuid.UUID `json:"-"`
}

type GoalRequest struct {
	Name      string    `json:"name" binding:"required,max=100"`
	Target    int64     `json:"target" binding:"gt=0"`
	Deadline  *string   `json:"deadline" binding:"omitnil,datetime=2006-01-02"`
	AccountID uuid.UUID `json:"account_id" binding:"required"`
}

func (r *GoalRequest) Normalize() { r.Name = strings.TrimSpace(r.Name) }

// Fill computes the progress fields from the saved balance on today.
func (g *Goal) Fill(saved int64, today time.Time) {
	g.Saved, g.Remaining = saved, max(g.Target-saved, 0)
	g.MonthlyNeeded, g.MonthsLeft = nil, nil
	if g.Deadline == nil {
		return
	}
	end, _ := time.Parse(time.DateOnly, *g.Deadline)
	months := (end.Year()-today.Year())*12 + int(end.Month()-today.Month())
	if end.Day() > today.Day() {
		months++ // a part month still counts as a chance to save
	}
	months = max(months, 1)
	step := int64(months) * 100                     // whole rupiah per month,
	needed := (g.Remaining + step - 1) / step * 100 // rounded up so the goal is reached on time
	g.MonthlyNeeded, g.MonthsLeft = &needed, &months
}

// InviteLink is a one-time link into a group.
type InviteLink struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}
