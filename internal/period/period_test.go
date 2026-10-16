package period

import (
	"testing"
	"time"
)

func TestFor(t *testing.T) {
	tests := []struct {
		date     string
		startDay int
		start    string
		end      string
		label    string
	}{
		{"2026-10-16", 25, "2026-09-25", "2026-10-24", "25 Sep – 24 Oct 2026"},
		{"2026-10-25", 25, "2026-10-25", "2026-11-24", "25 Oct – 24 Nov 2026"},
		{"2026-10-24", 25, "2026-09-25", "2026-10-24", "25 Sep – 24 Oct 2026"},
		{"2027-01-10", 25, "2026-12-25", "2027-01-24", "25 Dec 2026 – 24 Jan 2027"},
		{"2026-03-01", 28, "2026-02-28", "2026-03-27", "28 Feb – 27 Mar 2026"},
		{"2028-02-28", 28, "2028-02-28", "2028-03-27", "28 Feb – 27 Mar 2028"},
		{"2026-05-31", 1, "2026-05-01", "2026-05-31", "1 May – 31 May 2026"},
		{"2026-10-16", 10, "2026-10-10", "2026-11-09", "10 Oct – 9 Nov 2026"},
	}
	for _, tt := range tests {
		t.Run(tt.date, func(t *testing.T) {
			d, err := time.Parse(time.DateOnly, tt.date)
			if err != nil {
				t.Fatal(err)
			}
			p := For(d, tt.startDay)
			if got := p.Start.Format(time.DateOnly); got != tt.start {
				t.Errorf("Start = %s, want %s", got, tt.start)
			}
			if got := p.End.Format(time.DateOnly); got != tt.end {
				t.Errorf("End = %s, want %s", got, tt.end)
			}
			if want := p.End.AddDate(0, 0, 1); !p.Next.Equal(want) {
				t.Errorf("Next = %s, want %s", p.Next.Format(time.DateOnly), want.Format(time.DateOnly))
			}
			if got := p.Label(); got != tt.label {
				t.Errorf("Label = %q, want %q", got, tt.label)
			}
		})
	}
}
