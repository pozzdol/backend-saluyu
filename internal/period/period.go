package period

import (
	"fmt"
	"time"
)

// Period is a budgeting cycle. End is inclusive (for display); Next is End+1 day, the
// exclusive upper bound for SQL half-open ranges. All dates are midnight UTC.
type Period struct{ Start, End, Next time.Time } // End inclusive (display), Next = End+1 (SQL half-open); all midnight UTC

// Config is how a user's periods are cut.
type Config struct {
	Mode    string // payday (also ""), last_workday, calendar_month or weekly
	Day     int    // payday: 1..31; a day past the month's end falls on its last day
	Weekday int    // weekly: the first day of each week, 0 = Sunday
}

// For returns the period containing d.
func For(d time.Time, c Config) Period {
	y, m, day := d.Date()
	if c.Mode == "weekly" {
		today := time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
		start := today.AddDate(0, 0, -((int(today.Weekday()) - c.Weekday + 7) % 7))
		next := start.AddDate(0, 0, 7)
		return Period{Start: start, End: next.AddDate(0, 0, -1), Next: next}
	}
	start := c.startIn(y, m)
	if day < start.Day() {
		start = c.startIn(y, m-1)
	}
	next := c.startIn(start.Year(), start.Month()+1)
	return Period{Start: start, End: next.AddDate(0, 0, -1), Next: next}
}

// startIn is the day a monthly period starts in month m (normalized, so 0 and 13 work).
func (c Config) startIn(y int, m time.Month) time.Time {
	switch c.Mode {
	case "calendar_month":
		return ClampDay(y, m, 1)
	case "last_workday":
		return LastWorkday(y, m)
	}
	return ClampDay(y, m, c.Day)
}

// ClampDay is day `day` of month m, or the month's last day when it has fewer days.
func ClampDay(y int, m time.Month, day int) time.Time {
	first := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(max(day, 1), last), 0, 0, 0, 0, time.UTC)
}

// LastWorkday is the last Monday–Friday of month m.
// ponytail: public holidays are ignored; a holiday table would move pay a day earlier.
func LastWorkday(y int, m time.Month) time.Time {
	d := ClampDay(y, m, 31)
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

// Back is the period k periods before p.
func Back(p Period, k int, c Config) Period {
	for range k {
		p = For(p.Start.AddDate(0, 0, -1), c)
	}
	return p
}

// Label formats the period for display, e.g. "25 Sep – 24 Oct 2026".
func (p Period) Label() string {
	if p.Start.Year() == p.End.Year() {
		return fmt.Sprintf("%s – %s", p.Start.Format("2 Jan"), p.End.Format("2 Jan 2006"))
	}
	return fmt.Sprintf("%s – %s", p.Start.Format("2 Jan 2006"), p.End.Format("2 Jan 2006"))
}
