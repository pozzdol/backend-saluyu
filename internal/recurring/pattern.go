// Package recurring works out the dates a repeating transaction falls on.
package recurring

import "time"

// Pattern is how often a rule repeats. Day is the weekday (0 = Sunday) for weekly, and the
// day of the month for monthly and payday (payday passes the user's period_start_day).
// A day past the month's end (31 in February) falls on the month's last day.
type Pattern struct {
	Frequency string // daily, weekly, monthly, payday
	Day       int
}

// OnOrAfter is the first date the pattern falls on, d itself included.
func (p Pattern) OnOrAfter(d time.Time) time.Time {
	d = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	switch p.Frequency {
	case "weekly":
		return d.AddDate(0, 0, (p.Day-int(d.Weekday())+7)%7)
	case "monthly", "payday":
		if c := inMonth(d.Year(), d.Month(), p.Day); !c.Before(d) {
			return c
		}
		return inMonth(d.Year(), d.Month()+1, p.Day)
	}
	return d // daily
}

// Between lists the dates in [from, to].
func (p Pattern) Between(from, to time.Time) []time.Time {
	var out []time.Time
	for d := p.OnOrAfter(from); !d.After(to); d = p.OnOrAfter(d.AddDate(0, 0, 1)) {
		out = append(out, d)
	}
	return out
}

func inMonth(y int, m time.Month, day int) time.Time {
	first := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(day, last), 0, 0, 0, 0, time.UTC)
}
