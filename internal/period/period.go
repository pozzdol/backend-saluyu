package period

import (
	"fmt"
	"time"
)

// Period is a payday cycle. End is inclusive (for display); Next is End+1 day, the
// exclusive upper bound for SQL half-open ranges. All dates are midnight UTC.
type Period struct{ Start, End, Next time.Time } // End inclusive (display), Next = End+1 (SQL half-open); all midnight UTC

// For returns the payday period containing d. A period runs from startDay of one month to
// startDay-1 of the next. startDay is 1..28 so it exists in every month.
func For(d time.Time, startDay int) Period {
	y, m, day := d.Date()
	if day < startDay {
		m-- // time.Date normalizes month 0 to December of the previous year
	}
	start := time.Date(y, m, startDay, 0, 0, 0, 0, time.UTC)
	next := start.AddDate(0, 1, 0)
	return Period{Start: start, End: next.AddDate(0, 0, -1), Next: next}
}

// Label formats the period for display, e.g. "25 Sep – 24 Oct 2026".
func (p Period) Label() string {
	if p.Start.Year() == p.End.Year() {
		return fmt.Sprintf("%s – %s", p.Start.Format("2 Jan"), p.End.Format("2 Jan 2006"))
	}
	return fmt.Sprintf("%s – %s", p.Start.Format("2 Jan 2006"), p.End.Format("2 Jan 2006"))
}
