package recurring

import (
	"strings"
	"testing"
	"time"
)

func d(s string) time.Time { t, _ := time.Parse(time.DateOnly, s); return t }

func dates(ts []time.Time) string {
	s := make([]string, len(ts))
	for i, t := range ts {
		s[i] = t.Format("01-02")
	}
	return strings.Join(s, " ")
}

func TestBetween(t *testing.T) {
	for _, c := range []struct {
		p        Pattern
		from, to string
		want     string
	}{
		{Pattern{"daily", 0}, "2026-10-30", "2026-11-02", "10-30 10-31 11-01 11-02"},
		{Pattern{"weekly", 1}, "2026-10-01", "2026-10-31", "10-05 10-12 10-19 10-26"}, // Mondays
		{Pattern{"monthly", 31}, "2026-01-15", "2026-04-30", "01-31 02-28 03-31 04-30"},
		{Pattern{"payday", 25}, "2026-10-25", "2026-12-24", "10-25 11-25"},
		{Pattern{"monthly", 5}, "2026-10-06", "2026-10-31", ""},
	} {
		if got := dates(c.p.Between(d(c.from), d(c.to))); got != c.want {
			t.Errorf("%+v %s..%s: got %q want %q", c.p, c.from, c.to, got, c.want)
		}
	}
}
