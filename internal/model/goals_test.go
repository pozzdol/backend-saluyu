package model

import (
	"testing"
	"time"
)

func TestGoalFill(t *testing.T) {
	today := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	d := "2027-04-16"
	g := Goal{Target: 1_200_000_000, Deadline: &d} // Rp12 juta in 6 months
	g.Fill(600_000_000, today)
	if *g.MonthsLeft != 6 || *g.MonthlyNeeded != 100_000_000 || g.Remaining != 600_000_000 {
		t.Fatalf("%+v %d %d", g, *g.MonthsLeft, *g.MonthlyNeeded)
	}
	d = "2026-10-01" // past the deadline: everything left is due now
	g.Fill(0, today)
	if *g.MonthsLeft != 1 || *g.MonthlyNeeded != 1_200_000_000 {
		t.Fatalf("past deadline: %d %d", *g.MonthsLeft, *g.MonthlyNeeded)
	}
	g.Deadline = nil
	g.Fill(2_000_000_000, today)
	if g.Remaining != 0 || g.MonthlyNeeded != nil {
		t.Fatalf("reached, no deadline: %+v", g)
	}
}
