package handler

import (
	"fmt"
	"math"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// getData GETs path and returns body["data"] as an object.
func getData(t *testing.T, r *gin.Engine, tok, path string) map[string]any {
	t.Helper()
	code, body := call(t, r, "GET", path, tok, "")
	expect(t, "GET "+path, code, 200, body)
	return body["data"].(map[string]any)
}

// wantNum checks a JSON number field (sen) of obj.
func wantNum(t *testing.T, what string, obj map[string]any, field string, want int64) {
	t.Helper()
	if obj[field] != float64(want) {
		t.Fatalf("%s %s = %v, want %d (in %v)", what, field, obj[field], want, obj)
	}
}

// rowOf finds the row of accountID in a JSON array of rows.
func rowOf(t *testing.T, rows any, accountID string) map[string]any {
	t.Helper()
	for _, row := range rows.([]any) {
		if row := row.(map[string]any); row["account_id"] == accountID {
			return row
		}
	}
	t.Fatalf("no row for account %s in %v", accountID, rows)
	return nil
}

func post(t *testing.T, r *gin.Engine, tok, body string) map[string]any {
	t.Helper()
	code, out := call(t, r, "POST", "/api/v1/entries", tok, body)
	expect(t, "post "+body, code, 201, out)
	return out["data"].(map[string]any)
}

func TestDashboardAndReports(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "lida@x.com")
	acc := accountIDs(t, r, tok)
	bank, cash, tabungan, gaji, bunga, makan, jajan := acc["1001"], acc["1002"], acc["1003"], acc["4001"], acc["4002"], acc["5102"], acc["5103"]

	// Opening: Cash 5.000.000 dr / Ekuitas Saldo Awal 5.000.000 cr.
	code, body := call(t, r, "PUT", "/api/v1/opening", tok, fmt.Sprintf(
		`{"start_date":"2026-09-01","items":[{"account_id":%q,"amount":500000000}]}`, cash))
	expect(t, "opening", code, 200, body)

	post(t, r, tok, entryBody("expense", "2026-09-24", 1000000, bank, jajan)) // last day of the previous period
	post(t, r, tok, entryBody("income", "2026-09-25", 1000000000, gaji, bank))
	post(t, r, tok, entryBody("income", "2026-10-01", 5400000, bunga, bank))
	post(t, r, tok, entryBody("expense", "2026-10-05", 120000000, bank, makan))
	post(t, r, tok, entryBody("transfer", "2026-10-10", 200000000, bank, tabungan))

	code, body = call(t, r, "PUT", "/api/v1/budgets/"+makan, tok, `{"date":"2026-10-16","amount":150000000}`)
	expect(t, "set budget", code, 200, body)
	b := body["data"].(map[string]any)
	wantNum(t, "set budget", b, "budget", 150000000)
	wantNum(t, "set budget", b, "actual", 120000000)
	wantNum(t, "set budget", b, "remaining", 30000000)
	if b["over"] != false || b["inherited"] != false || b["name"] != "Makan" {
		t.Fatalf("set budget row: %v", b)
	}
	code, body = call(t, r, "PUT", "/api/v1/budgets/"+bank, tok, `{"date":"2026-10-16","amount":1}`)
	expect(t, "budget on an asset", code, 400, body)
	if body["error"] != "budgets are only for expense accounts" {
		t.Fatalf("budget on an asset: %v", body)
	}
	code, body = call(t, r, "PUT", "/api/v1/budgets/"+uuid.NewString(), tok, `{"date":"2026-10-16","amount":1}`)
	expect(t, "budget on unknown account", code, 404, body)
	code, body = call(t, r, "PUT", "/api/v1/budgets/"+makan, tok, `{"date":"2026-10-16","amount":-1}`)
	expect(t, "negative budget", code, 400, body)
	code, body = call(t, r, "GET", "/api/v1/dashboard?date=16-10-2026", tok, "")
	expect(t, "bad date", code, 400, body)
	if body["error"] != "invalid query parameter" || body["fields"].(map[string]any)["date"] != "is invalid" {
		t.Fatalf("bad date body: %v", body)
	}

	d := getData(t, r, tok, "/api/v1/dashboard?date=2026-10-16")
	p := d["period"].(map[string]any)
	if p["start"] != "2026-09-25" || p["end"] != "2026-10-24" || p["next"] != "2026-10-25" || p["prev"] != "2026-09-24" ||
		p["label"] != "25 Sep – 24 Oct 2026" {
		t.Fatalf("period: %v", p)
	}
	wantNum(t, "dashboard", d, "income", 1005400000) // Gaji 10.000.000 + Bunga 54.000
	wantNum(t, "dashboard", d, "expense", 120000000) // transfer to Tabungan and 24 Sep Jajan excluded
	wantNum(t, "dashboard", d, "surplus", 885400000)
	wantNum(t, "dashboard", d, "to_savings_total", 200000000)
	if rate := d["savings_rate"].(float64); math.Abs(rate-885400000.0/1005400000.0) > 1e-9 {
		t.Fatalf("savings_rate %v", rate)
	}
	if ts := d["to_savings"].([]any); len(ts) != 1 || ts[0].(map[string]any)["code"] != "1003" {
		t.Fatalf("to_savings: %v", ts)
	}
	if n := len(d["budgets"].([]any)); n != 8 {
		t.Fatalf("budgets has %d rows, want every active expense account (8)", n)
	}
	m := rowOf(t, d["budgets"], makan)
	wantNum(t, "makan budget", m, "remaining", 30000000)
	if m["over"] != false || m["inherited"] != false {
		t.Fatalf("makan budget: %v", m)
	}
	wantNum(t, "jajan budget", rowOf(t, d["budgets"], jajan), "budget", 0)
	// Bank on 24 Oct: 10.000.000 + 54.000 - 1.200.000 - 2.000.000 - 10.000 = 6.844.000
	wantNum(t, "bank balance", rowOf(t, d["balances"], bank), "balance", 684400000)
	wantNum(t, "cash balance", rowOf(t, d["balances"], cash), "balance", 500000000)
	wantNum(t, "tabungan balance", rowOf(t, d["balances"], tabungan), "balance", 200000000)

	bl := getData(t, r, tok, "/api/v1/budgets?date=2026-10-16")
	wantNum(t, "budget list", rowOf(t, bl["rows"], makan), "actual", 120000000)

	is := getData(t, r, tok, "/api/v1/reports/income-statement?date=2026-10-16")
	wantNum(t, "income statement", is, "revenue_total", 1005400000)
	wantNum(t, "income statement", is, "expense_total", 120000000)
	wantNum(t, "income statement", is, "net_income", 885400000) // equals the dashboard surplus
	if n := len(is["revenue"].([]any)); n != 2 {
		t.Fatalf("revenue rows %d, want 2 (zero rows dropped)", n)
	}

	bs := getData(t, r, tok, "/api/v1/reports/balance-sheet?date=2026-10-16")
	wantNum(t, "balance sheet", bs, "assets_total", 1384400000) // Bank 6.844.000 + Cash 5.000.000 + Tabungan 2.000.000
	wantNum(t, "balance sheet", bs, "liabilities_total", 0)
	wantNum(t, "balance sheet", bs, "equity_total", 500000000)
	wantNum(t, "balance sheet", bs, "current_earnings", 884400000) // all-time: 10.054.000 - 1.200.000 - 10.000
	if bs["as_of"] != "2026-10-16" || bs["balanced"] != true || len(bs["liabilities"].([]any)) != 0 {
		t.Fatalf("balance sheet: %v", bs)
	}

	// Next period: the Makan budget carries forward; spending 1.600.000 goes over it.
	post(t, r, tok, entryBody("expense", "2026-11-01", 160000000, bank, makan))
	m = rowOf(t, getData(t, r, tok, "/api/v1/dashboard?date=2026-11-01")["budgets"], makan)
	wantNum(t, "inherited budget", m, "budget", 150000000)
	wantNum(t, "inherited budget", m, "remaining", -10000000)
	if m["inherited"] != true || m["over"] != true {
		t.Fatalf("inherited budget: %v", m)
	}
}

func TestNextPeriod(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "lida@x.com")
	acc := accountIDs(t, r, tok)
	bank, gaji, makan := acc["1001"], acc["4001"], acc["5102"]
	openBooks(t, r, tok, "2026-09-01")

	// Salary arrives on 24 Oct, a day before payday, and belongs to the period starting 25 Oct.
	e := post(t, r, tok, fmt.Sprintf(`{"kind":"income","entry_date":"2026-10-24","amount":1000000000,"from_account_id":%q,"to_account_id":%q,"next_period":true}`, gaji, bank))
	if e["period_date"] != "2026-10-25" || e["entry_date"] != "2026-10-24" {
		t.Fatalf("next_period income: %v", e)
	}
	e = post(t, r, tok, fmt.Sprintf(
		`{"kind":"expense","entry_date":"2026-10-24","amount":100,"from_account_id":%q,"to_account_id":%q,"next_period":true}`, bank, makan))
	if e["period_date"] != nil {
		t.Fatalf("next_period must be ignored for expense: %v", e)
	}

	before := getData(t, r, tok, "/api/v1/dashboard?date=2026-10-16")
	wantNum(t, "Sep-Oct income", before, "income", 0)
	wantNum(t, "Sep-Oct bank on 24 Oct", rowOf(t, before["balances"], bank), "balance", 999999900) // the money is there already
	wantNum(t, "Oct-Nov income", getData(t, r, tok, "/api/v1/dashboard?date=2026-10-25"), "income", 1000000000)
	wantNum(t, "Sep-Oct net income", getData(t, r, tok, "/api/v1/reports/income-statement?date=2026-10-16"), "net_income", -100)

	bs := getData(t, r, tok, "/api/v1/reports/balance-sheet?date=2026-10-24")
	wantNum(t, "balance sheet 24 Oct", bs, "assets_total", 999999900)
	if bs["balanced"] != true {
		t.Fatalf("balance sheet: %v", bs)
	}

	code, body := call(t, r, "GET", "/api/v1/entries?kind=income", tok, "")
	expect(t, "list entries", code, 200, body)
	if pd := body["data"].([]any)[0].(map[string]any)["period_date"]; pd != "2026-10-25" {
		t.Fatalf("listed period_date %v", pd)
	}
}

func TestPeriodStartDay(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "lida@x.com")
	if day := getData(t, r, tok, "/api/v1/me")["period_start_day"]; day != float64(25) {
		t.Fatalf("default period_start_day %v, want 25", day)
	}

	code, body := call(t, r, "PATCH", "/api/v1/me", tok, `{"period_start_day":32}`)
	expect(t, "period_start_day 32", code, 400, body)
	if msg := body["fields"].(map[string]any)["period_start_day"]; msg != "must be at most 31" {
		t.Fatalf("period_start_day 32 message %q", msg)
	}
	code, body = call(t, r, "PATCH", "/api/v1/me", tok, `{"period_start_day":10}`)
	expect(t, "period_start_day 10", code, 200, body)
	if u := body["data"].(map[string]any); u["period_start_day"] != float64(10) || u["name"] != "Lida" {
		t.Fatalf("after update: %v", u)
	}

	p := getData(t, r, tok, "/api/v1/dashboard?date=2026-10-16")["period"].(map[string]any)
	if p["start"] != "2026-10-10" || p["end"] != "2026-11-09" || p["label"] != "10 Oct – 9 Nov 2026" {
		t.Fatalf("period with start day 10: %v", p)
	}
}
