package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func patchAccount(t *testing.T, r *gin.Engine, tok, id, body string) {
	t.Helper()
	code, out := call(t, r, "PATCH", "/api/v1/accounts/"+id, tok, body)
	expect(t, "patch account "+body, code, 200, out)
}

func budgetRow(t *testing.T, r *gin.Engine, tok, date, accountID string) map[string]any {
	t.Helper()
	return rowOf(t, getData(t, r, tok, "/api/v1/budgets?date="+date)["rows"], accountID)
}

func TestBudgetCarryOver(t *testing.T) {
	r, repo := newTestServer()
	tok := signUp(t, r, "carry@x.com")
	openBooks(t, r, tok, "2026-08-01")
	acc := accountIDs(t, r, tok)
	bank, makan, jajan := acc["1001"], acc["5102"], acc["5103"]

	for _, id := range []string{makan, jajan} {
		code, body := call(t, r, "PUT", "/api/v1/budgets/"+id, tok, `{"date":"2026-08-26","amount":1000000}`)
		expect(t, "set budget", code, 200, body)
	}
	// Only Makan is an envelope; the switch-on day is fixed to the first period for the test.
	patchAccount(t, r, tok, makan, `{"budget_carry":true}`)
	since := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	setCarrySince(repo, makan, since)

	post(t, r, tok, entryBody("expense", "2026-09-01", 300000, bank, makan)) // period 1: 700000 left
	post(t, r, tok, entryBody("expense", "2026-09-01", 300000, bank, jajan))
	post(t, r, tok, entryBody("expense", "2026-10-01", 1200000, bank, makan)) // period 2: overspends by 200000
	post(t, r, tok, entryBody("expense", "2026-10-01", 1200000, bank, jajan))
	post(t, r, tok, entryBody("expense", "2026-11-01", 100000, bank, makan)) // period 3

	// Period 1: nothing carried in yet.
	row := budgetRow(t, r, tok, "2026-09-01", makan)
	wantNum(t, "p1", row, "carry", 0)
	wantNum(t, "p1", row, "available", 1000000)
	// Period 2: 700000 carried; 1700000 available, so 1200000 spent is not over.
	row = budgetRow(t, r, tok, "2026-10-16", makan)
	wantNum(t, "p2", row, "carry", 700000)
	wantNum(t, "p2", row, "available", 1700000)
	wantNum(t, "p2", row, "remaining", 500000)
	if row["over"] != false || row["budget_carry"] != true {
		t.Fatalf("p2 row: %v", row)
	}
	// Period 3: +700000 - 200000, overspending reduced the next envelope.
	row = budgetRow(t, r, tok, "2026-11-01", makan)
	wantNum(t, "p3", row, "carry", 500000)
	wantNum(t, "p3", row, "available", 1500000)
	wantNum(t, "p3", row, "remaining", 1400000)
	// Jajan resets every period: same budget, no carry, and 1200000 > 1000000 is over.
	row = budgetRow(t, r, tok, "2026-10-16", jajan)
	wantNum(t, "jajan", row, "carry", 0)
	wantNum(t, "jajan", row, "available", 1000000)
	if row["over"] != true || row["budget_carry"] != false {
		t.Fatalf("jajan row: %v", row)
	}
	// The dashboard shows the same available amount.
	row = rowOf(t, getData(t, r, tok, "/api/v1/dashboard?date=2026-10-16")["budgets"], makan)
	wantNum(t, "dashboard", row, "available", 1700000)
}

func TestNeedGroupShares(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "groups@x.com")
	openBooks(t, r, tok, "2026-09-01")
	acc := accountIDs(t, r, tok)
	bank, tabungan, gaji, makan, jajan, hiburan := acc["1001"], acc["1003"], acc["4001"], acc["5102"], acc["5103"], acc["5104"]

	post(t, r, tok, entryBody("income", "2026-10-01", 1000000, gaji, bank))
	post(t, r, tok, entryBody("expense", "2026-10-02", 500000, bank, makan))
	post(t, r, tok, entryBody("expense", "2026-10-03", 200000, bank, jajan))
	post(t, r, tok, entryBody("expense", "2026-10-04", 100000, bank, hiburan))
	post(t, r, tok, entryBody("transfer", "2026-10-05", 200000, bank, tabungan))

	shares := func() map[string]map[string]any {
		d := getData(t, r, tok, "/api/v1/budgets?date=2026-10-16")
		wantNum(t, "budgets", d, "income", 1000000)
		out := map[string]map[string]any{}
		for _, g := range d["groups"].([]any) {
			g := g.(map[string]any)
			out[g["group"].(string)] = g
		}
		return out
	}
	g := shares()
	wantNum(t, "needs", g["needs"], "amount", 500000)
	wantNum(t, "wants", g["wants"], "amount", 300000)
	wantNum(t, "savings", g["savings"], "amount", 200000)
	if g["needs"]["share"] != 0.5 || g["wants"]["share"] != 0.3 || g["savings"]["share"] != 0.2 || g["needs"]["target"] != 0.5 {
		t.Fatalf("shares: %v", g)
	}
	// Seeded accounts arrive grouped; a user can move or clear them.
	if row := budgetRow(t, r, tok, "2026-10-16", makan); row["need_group"] != "needs" {
		t.Fatalf("makan group: %v", row)
	}
	patchAccount(t, r, tok, jajan, `{"need_group":"needs"}`)
	patchAccount(t, r, tok, hiburan, `{"need_group":"none"}`)
	g = shares()
	wantNum(t, "needs after move", g["needs"], "amount", 700000)
	wantNum(t, "wants after move", g["wants"], "amount", 0)
	code, body := call(t, r, "PATCH", "/api/v1/accounts/"+jajan, tok, `{"need_group":"bogus"}`)
	expect(t, "bad group", code, 400, body)
}

func rawGet(r *gin.Engine, tok, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestEntriesRunningBalanceAndExport(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "export@x.com")
	openBooks(t, r, tok, "2026-09-01")
	acc := accountIDs(t, r, tok)
	bank, cash, gaji, makan := acc["1001"], acc["1002"], acc["4001"], acc["5102"]

	post(t, r, tok, entryBody("income", "2026-10-01", 1000000, gaji, bank))
	post(t, r, tok, entryBody("expense", "2026-10-02", 400000, bank, makan))
	post(t, r, tok, entryBody("transfer", "2026-10-02", 100000, bank, cash)) // same day: entry_no breaks the tie
	post(t, r, tok, entryBody("income", "2026-10-03", 50000, gaji, cash))    // not on Bank

	// Bank, newest first: 500000 after the transfer, 600000 after the expense, 1000000 after the salary.
	code, body := call(t, r, "GET", "/api/v1/entries?account_id="+bank, tok, "")
	expect(t, "list bank", code, 200, body)
	var got []string
	for _, e := range body["data"].([]any) {
		got = append(got, fmt.Sprintf("%.0f", e.(map[string]any)["balance_after"]))
	}
	if strings.Join(got, ",") != "500000,600000,1000000" {
		t.Fatalf("running balance = %v", got)
	}
	// A later page keeps the balance of its own rows.
	code, body = call(t, r, "GET", "/api/v1/entries?account_id="+bank+"&limit=1&page=2", tok, "")
	expect(t, "page 2", code, 200, body)
	if e := body["data"].([]any)[0].(map[string]any); e["balance_after"] != float64(600000) {
		t.Fatalf("page 2 row: %v", e)
	}
	// Unfiltered lists carry no balance.
	_, body = call(t, r, "GET", "/api/v1/entries", tok, "")
	if _, has := body["data"].([]any)[0].(map[string]any)["balance_after"]; has {
		t.Fatalf("unfiltered list has balance_after: %v", body["data"])
	}

	// CSV: BOM, header, filtered rows; Saldo only with an account filter.
	w := rawGet(r, tok, "/api/v1/entries/export?format=csv&kind=expense")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("csv export: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	if !strings.HasPrefix(w.Body.String(), "\uFEFFNo,Tanggal,Jenis,Keterangan,Debit,Kredit,Jumlah\n") || len(lines) != 2 {
		t.Fatalf("csv = %q", w.Body.String())
	}
	if !strings.Contains(lines[1], "2026-10-02,expense,test,Makan,Bank,4000.00") {
		t.Fatalf("csv row = %q", lines[1])
	}
	w = rawGet(r, tok, "/api/v1/entries/export?account_id="+bank)
	lines = strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	if len(lines) != 4 || !strings.HasSuffix(lines[0], ",Saldo") || !strings.HasSuffix(lines[1], ",5000.00") || !strings.HasSuffix(lines[3], ",10000.00") {
		t.Fatalf("csv with balance = %q", w.Body.String())
	}
	// XLSX is a ZIP; a bad format is rejected.
	w = rawGet(r, tok, "/api/v1/entries/export?format=xlsx")
	if w.Code != 200 || !strings.HasPrefix(w.Body.String(), "PK") || !strings.Contains(w.Header().Get("Content-Disposition"), "transaksi.xlsx") {
		t.Fatalf("xlsx export: %d %q", w.Code, w.Header().Get("Content-Disposition"))
	}
	if w = rawGet(r, tok, "/api/v1/entries/export?format=pdf"); w.Code != http.StatusBadRequest {
		t.Fatalf("pdf export = %d", w.Code)
	}
}

// setCarrySince moves the switch-on day of an envelope into the past.
func setCarrySince(repo *fakeRepo, accountID string, since time.Time) {
	id := uuid.MustParse(accountID)
	for _, accs := range repo.ledger.accounts {
		if a, ok := accs[id]; ok {
			a.BudgetCarrySince = &since
		}
	}
}
