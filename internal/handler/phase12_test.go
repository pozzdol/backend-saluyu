package handler

import (
	"testing"
	"time"
)

// safeOf reads the insights' safe_per_day for date.
func (b *book) safeOf(date string) map[string]any {
	b.t.Helper()
	code, body := b.r("GET", "/api/v1/insights?date="+date, "")
	expect(b.t, "insights", code, 200, body)
	return body["data"].(map[string]any)["safe_per_day"].(map[string]any)
}

// A brand-new book with money but no income yet must not look like a failure (D17).
func TestPeriodMoneyFreshBook(t *testing.T) {
	today := time.Now().Format(time.DateOnly)
	b := newBook(t, "fresh12@x.com", today, "1001=300000000 1002=46000000 1003=100000000")

	safe := b.safeOf(today)
	// Savings (1003) stay out of the money to spend.
	if num(safe["opening_cash"]) != 346000000 || num(safe["period_money"]) != 346000000 || safe["deficit"] != false {
		t.Fatalf("fresh book: %v", safe)
	}
	if num(safe["amount"]) <= 0 || num(safe["remaining"]) != 346000000 || safe["basis"] != "period_money" {
		t.Fatalf("fresh book must have money to spend: %v", safe)
	}

	b.post("expense", today, 2500000, "1002", "5103")
	safe = b.safeOf(today)
	if safe["deficit"] != false || num(safe["remaining"]) != 343500000 {
		t.Fatalf("after one snack: %v", safe)
	}
	d := getDashboardOf(b, today)
	if num(d["period_money"]) != 346000000 || num(d["cash_total"]) != 443500000 || num(d["expense"]) != 2500000 {
		t.Fatalf("dashboard: period_money %v cash_total %v expense %v", d["period_money"], d["cash_total"], d["expense"])
	}

	// A daily reminder of Rp10.000 until the period ends is money already promised.
	days := num(safe["days_left"])
	code, body := b.r("POST", "/api/v1/recurring", `{"name":"Parkir","kind":"expense","from_account_id":"`+b.acc["1002"]+
		`","to_account_id":"`+b.acc["5101"]+`","amount":1000000,"frequency":"daily","mode":"remind","start_date":"`+today+`"}`)
	expect(t, "recurring", code, 201, body)
	safe = b.safeOf(today)
	if num(safe["scheduled"]) != 1000000*days || num(safe["remaining"]) != 343500000-1000000*days {
		t.Fatalf("recurring over %d days: %v", days, safe)
	}

	// A card purchase is card debt: counted once, cash stays the same.
	code, body = b.r("POST", "/api/v1/cards", `{"name":"Visa","kind":"credit_card","statement_day":20,"due_day":5,"credit_limit":0,"min_payment_pct":10}`)
	expect(t, "card", code, 201, body)
	b.acc["card"] = body["data"].(map[string]any)["account"].(map[string]any)["id"].(string)
	b.post("expense", today, 5000000, "card", "5102")
	after := b.safeOf(today)
	if num(after["cash"]) != 343500000 || num(after["remaining"]) != num(safe["remaining"])-5000000 {
		t.Fatalf("card purchase: before %v after %v", safe, after)
	}
}

// Without income in the period, the money that was there at its start is what it has.
func TestPeriodMoneyCarriesOver(t *testing.T) {
	now := time.Now()
	today := now.Format(time.DateOnly)
	b := newBook(t, "carry12@x.com", now.AddDate(0, -3, 0).Format(time.DateOnly), "1001=100000000")
	b.post("income", now.AddDate(0, -2, 0).Format(time.DateOnly), 50000000, "4001", "1001") // an earlier period
	safe := b.safeOf(today)
	if num(safe["opening_cash"]) != 150000000 || num(safe["period_money"]) != 150000000 || safe["deficit"] != false || num(safe["amount"]) <= 0 {
		t.Fatalf("carried over: %v", safe)
	}
	// Spending past the period's money (here on a card) is a deficit.
	code, body := b.r("POST", "/api/v1/cards", `{"name":"Visa","kind":"credit_card","statement_day":20,"due_day":5,"credit_limit":0,"min_payment_pct":10}`)
	expect(t, "card", code, 201, body)
	b.acc["card"] = body["data"].(map[string]any)["account"].(map[string]any)["id"].(string)
	b.post("expense", today, 200000000, "card", "5105")
	if safe = b.safeOf(today); safe["deficit"] != true || num(safe["amount"]) != 0 {
		t.Fatalf("overspent: %v", safe)
	}
}

func getDashboardOf(b *book, date string) map[string]any {
	b.t.Helper()
	code, body := b.r("GET", "/api/v1/dashboard?date="+date, "")
	expect(b.t, "dashboard", code, 200, body)
	return body["data"].(map[string]any)
}

func TestPeriodModes(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "modes12@x.com")
	me := getData(t, r, tok, "/api/v1/me")
	if me["period_mode"] != "payday" || me["period_weekday"] != float64(1) {
		t.Fatalf("defaults: %v", me)
	}
	tests := []struct{ body, date, start, end string }{
		{`{"period_start_day":31}`, "2026-02-15", "2026-01-31", "2026-02-27"},
		{`{"period_mode":"calendar_month"}`, "2026-10-19", "2026-10-01", "2026-10-31"},
		{`{"period_mode":"last_workday"}`, "2026-10-29", "2026-09-30", "2026-10-29"},
		{`{"period_mode":"weekly","period_weekday":1}`, "2026-10-21", "2026-10-19", "2026-10-25"},
	}
	for _, tt := range tests {
		code, body := call(t, r, "PATCH", "/api/v1/me", tok, tt.body)
		expect(t, tt.body, code, 200, body)
		p := getData(t, r, tok, "/api/v1/dashboard?date="+tt.date)["period"].(map[string]any)
		if p["start"] != tt.start || p["end"] != tt.end {
			t.Fatalf("%s: period %v, want %s – %s", tt.body, p, tt.start, tt.end)
		}
	}
	code, body := call(t, r, "PATCH", "/api/v1/me", tok, `{"period_mode":"fortnightly"}`)
	expect(t, "bad mode", code, 400, body)
	code, body = call(t, r, "PATCH", "/api/v1/me", tok, `{"period_weekday":7}`)
	expect(t, "bad weekday", code, 400, body)
}

// New books get Indonesian names; system categories report whether they were ever used.
func TestAccountsSeedAndUse(t *testing.T) {
	today := time.Now().Format(time.DateOnly)
	b := newBook(t, "seed12@x.com", today, "1002=1000000")
	_, body := b.r("GET", "/api/v1/accounts", "")
	used := func() map[string]any {
		_, body := b.r("GET", "/api/v1/accounts", "")
		out := map[string]any{}
		for _, a := range body["data"].([]any) {
			a := a.(map[string]any)
			out[a["code"].(string)] = a["has_entries"]
		}
		return out
	}
	names := map[string]string{}
	for _, a := range body["data"].([]any) {
		a := a.(map[string]any)
		names[a["code"].(string)] = a["name"].(string)
	}
	if names["1002"] != "Tunai" || names["1004"] != "Dompet Digital" || names["5107"] != "Biaya Admin Bank" {
		t.Fatalf("seed names: %v", names)
	}
	if u := used(); u["5199"] != false || u["5108"] != false || u["1002"] != true {
		t.Fatalf("before use: %v", u)
	}
	b.post("expense", today, 100000, "1002", "5199")
	if u := used(); u["5199"] != true {
		t.Fatalf("after use: %v", u)
	}
}
