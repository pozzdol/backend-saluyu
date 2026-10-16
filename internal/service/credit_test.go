package service

import (
	"testing"
	"time"
)

func day(s string) time.Time { t, _ := time.Parse(time.DateOnly, s); return t }

func TestCycleDates(t *testing.T) {
	for _, c := range []struct {
		stmt, dueDay                 int
		today, prev, last, next, due string
	}{
		// Statement on the 20th, due on the 5th of the next month.
		{20, 5, "2026-10-16", "2026-08-20", "2026-09-20", "2026-10-20", "2026-10-05"},
		{20, 5, "2026-10-20", "2026-09-20", "2026-10-20", "2026-11-20", "2026-11-05"},
		// Due later in the same month as the statement.
		{3, 18, "2026-10-16", "2026-09-03", "2026-10-03", "2026-11-03", "2026-10-18"},
		// Across the new year.
		{28, 15, "2027-01-10", "2026-11-28", "2026-12-28", "2027-01-28", "2027-01-15"},
	} {
		prev, last, next, due := CycleDates(c.stmt, c.dueDay, day(c.today))
		got := [4]string{prev.Format(time.DateOnly), last.Format(time.DateOnly), next.Format(time.DateOnly), due.Format(time.DateOnly)}
		if got != [4]string{c.prev, c.last, c.next, c.due} {
			t.Errorf("stmt %d due %d on %s: got %v", c.stmt, c.dueDay, c.today, got)
		}
	}
}

func TestMinimumPayment(t *testing.T) {
	for _, c := range []struct{ total, want int64 }{
		{0, 0},
		{3_000_000, 3_000_000},    // Rp30.000 bill: pay it all
		{20_000_000, 5_000_000},   // 10% is Rp20.000, the floor Rp50.000 wins
		{300_000_000, 30_000_000}, // 10% of Rp3.000.000
		{123_456_789, 12_345_679}, // rounds to the sen
	} {
		if got := MinimumPayment(c.total, 10); got != c.want {
			t.Errorf("total %d: got %d want %d", c.total, got, c.want)
		}
	}
}
