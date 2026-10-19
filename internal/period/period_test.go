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
			p := For(d, Config{Day: tt.startDay})
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

func TestForModes(t *testing.T) {
	tests := []struct {
		name  string
		date  string
		cfg   Config
		start string
		end   string
	}{
		// Payday on the 31st falls on the month's last day.
		{"payday 31 in Feb", "2026-02-15", Config{Day: 31}, "2026-01-31", "2026-02-27"},
		{"payday 31 from Feb 28", "2026-03-10", Config{Day: 31}, "2026-02-28", "2026-03-30"},
		{"payday 30 in Apr", "2026-04-30", Config{Day: 30}, "2026-04-30", "2026-05-29"},
		{"payday 29 leap", "2028-02-29", Config{Day: 29}, "2028-02-29", "2028-03-28"},
		{"calendar month", "2026-10-19", Config{Mode: "calendar_month", Day: 25}, "2026-10-01", "2026-10-31"},
		{"calendar Dec", "2026-12-31", Config{Mode: "calendar_month"}, "2026-12-01", "2026-12-31"},
		// 31 Oct 2026 is a Saturday, so pay comes on Friday 30 Oct; 30 Sep 2026 is a Wednesday.
		{"last workday before", "2026-10-29", Config{Mode: "last_workday"}, "2026-09-30", "2026-10-29"},
		{"last workday on", "2026-10-30", Config{Mode: "last_workday"}, "2026-10-30", "2026-11-29"},
		{"last workday year end", "2027-01-05", Config{Mode: "last_workday"}, "2026-12-31", "2027-01-28"},
		// 19 Oct 2026 is a Monday.
		{"weekly monday", "2026-10-19", Config{Mode: "weekly", Weekday: 1}, "2026-10-19", "2026-10-25"},
		{"weekly monday sunday", "2026-10-25", Config{Mode: "weekly", Weekday: 1}, "2026-10-19", "2026-10-25"},
		{"weekly saturday", "2026-10-19", Config{Mode: "weekly", Weekday: 6}, "2026-10-17", "2026-10-23"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, _ := time.Parse(time.DateOnly, tt.date)
			p := For(d, tt.cfg)
			if got := p.Start.Format(time.DateOnly); got != tt.start {
				t.Errorf("Start = %s, want %s", got, tt.start)
			}
			if got := p.End.Format(time.DateOnly); got != tt.end {
				t.Errorf("End = %s, want %s", got, tt.end)
			}
			if !p.Next.Equal(p.End.AddDate(0, 0, 1)) {
				t.Errorf("Next = %s", p.Next.Format(time.DateOnly))
			}
			// Back(For(x), 1) must end the day before, whatever the mode.
			if prev := Back(p, 1, tt.cfg); !prev.Next.Equal(p.Start) {
				t.Errorf("Back ends %s, want before %s", prev.End.Format(time.DateOnly), tt.start)
			}
		})
	}
}
