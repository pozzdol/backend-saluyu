// Package installment builds repayment schedules for loans with interest (KPR, motor credit,
// personal loans). Amounts are sen; every row is rounded to the sen and the last row absorbs
// the rounding, so the principal column always adds up to exactly the loan.
package installment

import (
	"math"
	"time"
)

type Method string

const (
	Flat    Method = "flat"    // interest on the original principal every month
	Annuity Method = "annuity" // equal payments; interest on the remaining principal
)

type Row struct {
	Seq       int       `json:"seq"`
	DueDate   time.Time `json:"-"`
	Due       string    `json:"due_date"`
	Principal int64     `json:"principal"`
	Interest  int64     `json:"interest"`
	Payment   int64     `json:"payment"`
	Remaining int64     `json:"remaining"` // principal still owed after this payment
}

// Schedule returns n rows. annualRateBP is the yearly rate in basis points (1050 = 10.50%).
func Schedule(principal int64, annualRateBP, n int, method Method, firstDue time.Time) []Row {
	i := float64(annualRateBP) / 10000 / 12
	var payment float64 // annuity only
	if method == Annuity && i > 0 {
		payment = float64(principal) * i / (1 - math.Pow(1+i, -float64(n)))
	}
	rows := make([]Row, n)
	left := principal
	for k := range n {
		var interest, part int64
		switch {
		case method == Flat:
			interest = round(float64(principal) * i)
			part = round(float64(principal) / float64(n))
		case i == 0:
			part = round(float64(principal) / float64(n))
		default:
			interest = round(float64(left) * i)
			part = round(payment) - interest
		}
		if k == n-1 || part > left {
			part = left // the last row takes whatever rounding left over
		}
		left -= part
		due := AddMonths(firstDue, k)
		rows[k] = Row{Seq: k + 1, DueDate: due, Due: due.Format(time.DateOnly), Principal: part, Interest: interest, Payment: part + interest, Remaining: left}
	}
	return rows
}

func round(x float64) int64 { return int64(math.Round(x)) }

// AddMonths moves a date by whole months, clamping the day to the month's end: 31 Jan + 1 = 28/29 Feb.
func AddMonths(d time.Time, months int) time.Time {
	y, m, day := d.Date()
	first := time.Date(y, m+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(day, last), 0, 0, 0, 0, time.UTC)
}
