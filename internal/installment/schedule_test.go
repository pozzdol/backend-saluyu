package installment

import (
	"testing"
	"time"
)

func date(s string) time.Time { t, _ := time.Parse(time.DateOnly, s); return t }

func sum(rows []Row) (principal, interest int64) {
	for _, r := range rows {
		principal += r.Principal
		interest += r.Interest
	}
	return
}

func TestAnnuity(t *testing.T) {
	// Rp1.000.000 at 12% a year (1% a month) over 12 months: the textbook payment is Rp88.848,79.
	rows := Schedule(100_000_000, 1200, 12, Annuity, date("2026-01-31"))
	first, last := rows[0], rows[11]
	if first.Payment != 8_884_879 || first.Interest != 1_000_000 || first.Principal != 7_884_879 {
		t.Fatalf("month 1: %+v", first)
	}
	p, i := sum(rows)
	if p != 100_000_000 || last.Remaining != 0 {
		t.Fatalf("principal must add up exactly: %d, last %+v", p, last)
	}
	// 12 × 88.848,79 = 1.066.185,48; rounding may move the last payment by a few sen.
	if total := p + i; total < 106_618_540 || total > 106_618_556 {
		t.Fatalf("total paid %d", total)
	}
	if last.Payment < 8_884_870 || last.Payment > 8_884_890 {
		t.Fatalf("last payment drifts: %+v", last)
	}
	if rows[1].Due != "2026-02-28" || rows[2].Due != "2026-03-31" {
		t.Fatalf("due dates clamp to month end and come back: %s %s", rows[1].Due, rows[2].Due)
	}
}

func TestFlat(t *testing.T) {
	// Motor credit: Rp18.000.000 at 9% flat a year, 36 months.
	rows := Schedule(1_800_000_000, 900, 36, Flat, date("2026-03-10"))
	for _, r := range rows[:35] {
		if r.Principal != 50_000_000 || r.Interest != 13_500_000 {
			t.Fatalf("row %d: %+v", r.Seq, r)
		}
	}
	p, i := sum(rows)
	if p != 1_800_000_000 || i != 486_000_000 {
		t.Fatalf("totals %d %d", p, i)
	}
}

func TestZeroRateAndRemainder(t *testing.T) {
	// Rp100.000 over 3 months at 0%: 33.333,33 twice, then the remaining 33.333,34.
	rows := Schedule(10_000_000, 0, 3, Annuity, date("2026-01-15"))
	if rows[0].Principal != 3_333_333 || rows[2].Principal != 3_333_334 || rows[2].Remaining != 0 {
		t.Fatalf("%+v", rows)
	}
	for _, r := range rows {
		if r.Interest != 0 {
			t.Fatalf("no interest at 0%%: %+v", r)
		}
	}
}

func TestAddMonths(t *testing.T) {
	for _, c := range []struct {
		from string
		n    int
		want string
	}{{"2024-01-31", 1, "2024-02-29"}, {"2025-01-31", 1, "2025-02-28"}, {"2026-12-15", 1, "2027-01-15"}, {"2026-05-31", 12, "2027-05-31"}} {
		if got := AddMonths(date(c.from), c.n).Format(time.DateOnly); got != c.want {
			t.Errorf("%s + %d = %s, want %s", c.from, c.n, got, c.want)
		}
	}
}
