package handler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
)

func (f *fakeLedger) EntryLines(_ context.Context, userID uuid.UUID, from, to time.Time) ([]model.EntryLine, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	entries := append([]*model.Entry{}, f.entries[userID]...)
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].EntryDate.Before(entries[j].EntryDate) })
	var out []model.EntryLine
	for _, e := range entries {
		if e.EntryDate.Before(from) || !e.EntryDate.Before(to) {
			continue
		}
		for _, l := range e.Lines {
			a := f.userAccounts(userID)[l.AccountID]
			out = append(out, model.EntryLine{EntryID: e.ID, EntryNo: e.EntryNo, Description: e.Description, Kind: e.Kind, Date: e.EntryDate,
				AccountID: a.ID, Code: a.Code, Name: a.Name, Type: a.Type, Category: a.Category, Debit: l.Debit, Credit: l.Credit})
		}
	}
	return out, nil
}

type book struct {
	t   *testing.T
	r   func(method, path, body string) (int, map[string]any)
	acc map[string]string
}

func newBook(t *testing.T, email, start string, opening string) *book {
	r, _ := newTestServer()
	tok := signUp(t, r, email)
	b := &book{t: t, r: func(method, path, body string) (int, map[string]any) { return call(t, r, method, path, tok, body) }, acc: map[string]string{}}
	_, body := b.r("GET", "/api/v1/accounts", "")
	for _, a := range body["data"].([]any) {
		a := a.(map[string]any)
		b.acc[a["code"].(string)] = a["id"].(string)
	}
	// opening is "code=sen code=sen ...".
	var items []string
	for _, pair := range strings.Fields(opening) {
		code, amount, _ := strings.Cut(pair, "=")
		items = append(items, fmt.Sprintf(`{"account_id":"%s","amount":%s}`, b.acc[code], amount))
	}
	code, out := b.r("PUT", "/api/v1/opening", `{"start_date":"`+start+`","items":[`+strings.Join(items, ",")+`]}`)
	expect(t, "open books", code, 200, out)
	return b
}

// post records a simple entry: kind, date, amount (sen), from code, to code.
func (b *book) post(kind, date string, amount int64, from, to string) {
	b.t.Helper()
	code, body := b.r("POST", "/api/v1/entries", fmt.Sprintf(`{"kind":"%s","entry_date":"%s","amount":%d,"from_account_id":"%s","to_account_id":"%s"}`,
		kind, date, amount, b.acc[from], b.acc[to]))
	expect(b.t, kind+" "+date, code, 201, body)
}

func TestCashFlowMatchesCash(t *testing.T) {
	// Payday 25: the period of 2026-03-10 is 25 Feb – 24 Mar.
	b := newBook(t, "cf@x.com", "2026-01-01", "1001=1000000000 1002=50000000 1005=100000000")
	b.post("expense", "2026-02-10", 2000000, "1002", "5102") // before the period
	b.post("income", "2026-02-25", 1000000000, "4001", "1001")
	b.post("expense", "2026-03-01", 5000000, "1002", "5102")
	b.post("transfer", "2026-03-02", 200000000, "1001", "1005") // to investments: investing
	b.post("transfer", "2026-03-03", 20000000, "1001", "1002")  // cash to cash: nothing
	code, body := b.r("POST", "/api/v1/cards", `{"name":"Visa","kind":"credit_card","statement_day":20,"due_day":5,"credit_limit":0,"min_payment_pct":10}`)
	expect(t, "card", code, 201, body)
	card := body["data"].(map[string]any)["account"].(map[string]any)
	b.acc["card"] = card["id"].(string)
	b.post("expense", "2026-03-04", 30000000, "card", "5103")  // no cash moves
	b.post("transfer", "2026-03-05", 30000000, "1001", "card") // paying the card: financing
	code, body = b.r("POST", "/api/v1/installments", `{"name":"KTA","principal":500000000,"annual_rate":12,"tenor_months":12,"method":"annuity","first_due_date":"2026-04-05","received_into_id":"`+b.acc["1001"]+`","entry_date":"2026-03-06"}`)
	expect(t, "loan", code, 201, body)

	_, body = b.r("GET", "/api/v1/reports/cash-flow?date=2026-03-10", "")
	cf := body["data"].(map[string]any)
	section := func(name string) int64 { return num(cf[name].(map[string]any)["total"]) }
	if num(cf["beginning"]) != 1050000000-2000000 {
		t.Fatalf("beginning %v", cf["beginning"])
	}
	if section("operating") != 1000000000-5000000 || section("investing") != -200000000 || section("financing") != 500000000-30000000 {
		t.Fatalf("sections %v %v %v", section("operating"), section("investing"), section("financing"))
	}
	// The cash on the balance sheet at the period's end is exactly where the cash flow lands.
	_, body = b.r("GET", "/api/v1/reports/balance-sheet?date=2026-03-24", "")
	var cash int64
	for _, a := range body["data"].(map[string]any)["assets"].([]any) {
		a := a.(map[string]any)
		if a["code"] != "1005" && a["code"] != "1101" {
			cash += num(a["amount"])
		}
	}
	if num(cf["ending"]) != cash || num(cf["net"]) != cash-num(cf["beginning"]) {
		t.Fatalf("ending %v net %v, cash on the balance sheet %d", cf["ending"], cf["net"], cash)
	}
}

func TestInsightsMatchReports(t *testing.T) {
	today := time.Now()
	iso := func(d time.Time) string { return d.Format(time.DateOnly) }
	b := newBook(t, "ins@x.com", iso(today.AddDate(0, -5, 0)), "1001=1000000000")
	// Previous period: Makan 100.000; this period: Makan 300.000 and Jajan 50.000.
	prevDay := today.AddDate(0, -1, 0)
	b.post("expense", iso(prevDay), 10000000, "1001", "5102")
	b.post("expense", iso(today), 30000000, "1001", "5102")
	b.post("expense", iso(today), 5000000, "1001", "5103")
	code, body := b.r("PUT", "/api/v1/budgets/"+b.acc["5102"], `{"date":"`+iso(today)+`","amount":35000000}`)
	expect(t, "budget", code, 200, body)

	_, body = b.r("GET", "/api/v1/insights?date="+iso(today), "")
	ins := body["data"].(map[string]any)
	_, is := b.r("GET", "/api/v1/reports/income-statement?date="+iso(today)+"&compare=1", "")
	var donut int64
	for _, c := range ins["by_category"].([]any) {
		donut += num(c.(map[string]any)["amount"])
	}
	if donut != num(is["data"].(map[string]any)["expense_total"]) || donut != 35000000 {
		t.Fatalf("donut %d vs income statement %v", donut, is["data"].(map[string]any)["expense_total"])
	}
	var daily int64
	for _, d := range ins["daily"].([]any) {
		daily += num(d.(map[string]any)["expense"])
	}
	if daily != donut {
		t.Fatalf("daily total %d, donut %d", daily, donut)
	}
	alerts := ins["budget_alerts"].([]any)
	if len(alerts) != 1 || alerts[0].(map[string]any)["level"] != "warn" {
		t.Fatalf("Makan at 86%% of budget should warn: %v", alerts)
	}
	safe := ins["safe_per_day"].(map[string]any)
	if safe["basis"] != "budget" || num(safe["remaining"]) != 5000000 || num(safe["days_left"]) < 1 {
		t.Fatalf("safe per day %v", safe)
	}
	up := ins["insights"].([]any)
	if len(up) == 0 || up[0].(map[string]any)["kind"] != "category_up" || num(up[0].(map[string]any)["amount"]) != 20000000 {
		t.Fatalf("Makan tripled: %v", up)
	}
	row := is["data"].(map[string]any)["expense"].([]any)[0].(map[string]any)
	if num(row["prev"]) != 10000000 {
		t.Fatalf("compare prev %v", row)
	}
}

func TestHealth(t *testing.T) {
	today := time.Now()
	iso := func(d time.Time) string { return d.Format(time.DateOnly) }
	b := newBook(t, "health@x.com", iso(today.AddDate(0, -4, -5)), "1001=7000000000")
	for k := 1; k <= 3; k++ {
		d := today.AddDate(0, -k, 0)
		b.post("income", iso(d), 1000000000, "4001", "1001")
		b.post("expense", iso(d), 700000000, "1001", "5105")
	}
	_, body := b.r("GET", "/api/v1/health?date="+iso(today), "")
	h := body["data"].(map[string]any)
	ratio := func(k string) map[string]any { return h[k].(map[string]any) }
	if v := ratio("savings_3")["value"].(float64); v != 0.3 || ratio("savings_3")["status"] != "good" {
		t.Fatalf("savings 3: %v", ratio("savings_3"))
	}
	// Cash 70M + 3×(10M − 7M) = 79M over 7M a period: about 11 periods, good.
	if ratio("emergency_months")["status"] != "good" || num(h["avg_expense"]) != 700000000 {
		t.Fatalf("emergency %v avg %v", ratio("emergency_months"), h["avg_expense"])
	}
	if ratio("savings_12")["status"] != "none" {
		t.Fatalf("12 periods of data do not exist yet: %v", ratio("savings_12"))
	}
}
