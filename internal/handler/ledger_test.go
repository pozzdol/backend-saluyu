package handler

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/repository"
)

// fakeLedger is an in-memory service.LedgerRepository. Like register in production,
// a user gets the default chart of accounts the first time the ledger sees them.
type fakeLedger struct {
	mu       sync.Mutex
	accounts map[uuid.UUID]map[uuid.UUID]*model.Account // user -> id -> account
	entries  map[uuid.UUID][]*model.Entry               // user -> entries
	start    map[uuid.UUID]time.Time                    // user -> start date
}

func newFakeLedger() *fakeLedger {
	return &fakeLedger{accounts: map[uuid.UUID]map[uuid.UUID]*model.Account{},
		entries: map[uuid.UUID][]*model.Entry{}, start: map[uuid.UUID]time.Time{}}
}

func (f *fakeLedger) StartDate(_ context.Context, userID uuid.UUID) (*time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if d, ok := f.start[userID]; ok {
		return &d, nil
	}
	return nil, nil
}

func (f *fakeLedger) BalanceAsOf(_ context.Context, userID, accountID uuid.UUID, date time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var b int64
	for _, e := range f.entries[userID] {
		if e.EntryDate.After(date) {
			continue
		}
		for _, l := range e.Lines {
			if l.AccountID == accountID {
				b += l.Debit - l.Credit
			}
		}
	}
	return b, nil
}

func (f *fakeLedger) ReplaceOpening(_ context.Context, userID uuid.UUID, startDate time.Time, e *model.Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	kept := []*model.Entry{}
	for _, old := range f.entries[userID] {
		if old.Kind != "opening" {
			return repository.ErrBooksNotEmpty
		}
	}
	if e != nil {
		e.ID, e.EntryNo, e.CreatedAt = uuid.New(), 1, time.Now()
		kept = append(kept, e)
	}
	f.entries[userID] = kept
	f.start[userID] = startDate
	return nil
}

func (f *fakeLedger) userAccounts(userID uuid.UUID) map[uuid.UUID]*model.Account {
	if acc, ok := f.accounts[userID]; ok {
		return acc
	}
	acc := map[uuid.UUID]*model.Account{}
	for _, t := range model.DefaultAccounts {
		id := uuid.New()
		acc[id] = &model.Account{ID: id, Code: t.Code, Name: t.Name, Type: t.Type, Category: t.Category,
			NormalBalance: model.NormalBalance(t.Type), IsActive: true}
	}
	f.accounts[userID] = acc
	return acc
}

func (f *fakeLedger) ListAccounts(_ context.Context, userID uuid.UUID) ([]model.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.Account{}
	for _, a := range f.userAccounts(userID) {
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func (f *fakeLedger) FindAccounts(_ context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]model.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[uuid.UUID]model.Account{}
	for _, id := range ids {
		if a, ok := f.userAccounts(userID)[id]; ok {
			out[id] = *a
		}
	}
	return out, nil
}

func (f *fakeLedger) CreateAccount(_ context.Context, userID uuid.UUID, a *model.Account) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, other := range f.userAccounts(userID) {
		if other.Code == a.Code {
			return repository.ErrAccountCodeTaken
		}
	}
	a.ID, a.IsActive = uuid.New(), true
	copied := *a
	f.userAccounts(userID)[a.ID] = &copied
	return nil
}

func (f *fakeLedger) UpdateAccount(_ context.Context, userID, id uuid.UUID, req model.UpdateAccountRequest) (*model.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.userAccounts(userID)[id]
	if !ok {
		return nil, repository.ErrAccountNotFound
	}
	if req.Name != nil {
		a.Name = *req.Name
	}
	if req.IsActive != nil {
		a.IsActive = *req.IsActive
	}
	copied := *a
	return &copied, nil
}

func (f *fakeLedger) CreateEntry(_ context.Context, userID uuid.UUID, e *model.Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e.ID, e.EntryNo, e.CreatedAt = uuid.New(), int64(len(f.entries[userID])+1), time.Now()
	f.entries[userID] = append(f.entries[userID], e)
	return nil
}

func (f *fakeLedger) DeleteEntry(_ context.Context, userID, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, e := range f.entries[userID] {
		if e.ID == id && e.Kind == "opening" {
			return repository.ErrOpeningLocked
		}
		if e.ID == id {
			f.entries[userID] = append(f.entries[userID][:i], f.entries[userID][i+1:]...)
			return nil
		}
	}
	return repository.ErrEntryNotFound
}

// ListEntries honors only kind and limit: SQL filtering is not what these tests prove.
func (f *fakeLedger) ListEntries(_ context.Context, userID uuid.UUID, filter model.EntryFilter) ([]model.Entry, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.Entry{}
	for _, e := range f.entries[userID] {
		if filter.Kind == nil || e.Kind == *filter.Kind {
			out = append(out, *e)
		}
	}
	total := len(out)
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, total, nil
}

func (f *fakeLedger) LedgerLines(_ context.Context, userID, accountID uuid.UUID, from, _ *time.Time) (int64, []model.LedgerRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var opening int64
	rows := []model.LedgerRow{}
	for _, e := range f.entries[userID] {
		for _, l := range e.Lines {
			if l.AccountID != accountID {
				continue
			}
			if from != nil && e.EntryDate.Before(*from) {
				opening += l.Debit - l.Credit
				continue
			}
			rows = append(rows, model.LedgerRow{EntryID: e.ID, EntryNo: e.EntryNo, Date: e.Date, Debit: l.Debit, Credit: l.Credit})
		}
	}
	return opening, rows, nil
}

func (f *fakeLedger) TrialBalanceTotals(_ context.Context, userID uuid.UUID, _ time.Time) ([]model.TrialBalanceRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	totals := map[uuid.UUID]*model.TrialBalanceRow{}
	for id, a := range f.userAccounts(userID) {
		totals[id] = &model.TrialBalanceRow{AccountID: id, Code: a.Code, Name: a.Name, Type: a.Type}
	}
	for _, e := range f.entries[userID] {
		for _, l := range e.Lines {
			totals[l.AccountID].Debit += l.Debit
			totals[l.AccountID].Credit += l.Credit
		}
	}
	out := []model.TrialBalanceRow{}
	for _, t := range totals {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

// signUp registers and logs in a user and returns their access token.
func signUp(t *testing.T, r *gin.Engine, email string) string {
	t.Helper()
	call(t, r, "POST", "/api/v1/register", "", `{"name":"Lida","email":"`+email+`","password":"secret123"}`)
	code, body := call(t, r, "POST", "/api/v1/login", "", `{"email":"`+email+`","password":"secret123"}`)
	expect(t, "login "+email, code, 200, body)
	return body["access_token"].(string)
}

// openBooks sets an all-zero opening balance so the user can post from date on.
func openBooks(t *testing.T, r *gin.Engine, tok, date string) {
	t.Helper()
	code, body := call(t, r, "PUT", "/api/v1/opening", tok, `{"start_date":"`+date+`","items":[]}`)
	expect(t, "open books", code, 200, body)
}

// accountIDs maps account code to ID for the logged-in user.
func accountIDs(t *testing.T, r *gin.Engine, tok string) map[string]string {
	t.Helper()
	code, body := call(t, r, "GET", "/api/v1/accounts", tok, "")
	expect(t, "list accounts", code, 200, body)
	ids := map[string]string{}
	for _, a := range body["data"].([]any) {
		a := a.(map[string]any)
		ids[a["code"].(string)] = a["id"].(string)
	}
	return ids
}

func entryBody(kind, date string, amount int64, from, to string) string {
	return fmt.Sprintf(`{"kind":%q,"entry_date":%q,"amount":%d,"from_account_id":%q,"to_account_id":%q,"description":" test "}`,
		kind, date, amount, from, to)
}

func TestLedgerFlow(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "lida@x.com")
	acc := accountIDs(t, r, tok)
	if len(acc) != len(model.DefaultAccounts) {
		t.Fatalf("new user has %d accounts, want %d", len(acc), len(model.DefaultAccounts))
	}
	bank, tabungan, gaji, makan := acc["1001"], acc["1003"], acc["4001"], acc["5102"]
	openBooks(t, r, tok, "2026-09-01")

	// Salary 10,826,012.92: sen keep the cents exact.
	code, body := call(t, r, "POST", "/api/v1/entries", tok, entryBody("income", "2026-09-25", 1082601292, gaji, bank))
	expect(t, "income", code, 201, body)
	lines := body["data"].(map[string]any)["lines"].([]any)
	dr, cr := lines[0].(map[string]any), lines[1].(map[string]any)
	if dr["code"] != "1001" || dr["debit"] != float64(1082601292) || cr["code"] != "4001" || cr["credit"] != float64(1082601292) {
		t.Fatalf("income must be Dr Bank / Cr Gaji, got %v", lines)
	}
	if body["data"].(map[string]any)["description"] != "test" {
		t.Fatalf("description not trimmed: %v", body["data"])
	}

	code, body = call(t, r, "POST", "/api/v1/entries", tok, entryBody("expense", "2026-09-26", 3000000, bank, makan))
	expect(t, "expense", code, 201, body)
	code, body = call(t, r, "POST", "/api/v1/entries", tok, entryBody("transfer", "2026-09-27", 500000000, bank, tabungan))
	expect(t, "transfer", code, 201, body)
	if body["data"].(map[string]any)["entry_no"] != float64(3) {
		t.Fatalf("entry_no should count per user, got %v", body["data"])
	}

	// Wrong direction or type: expense must go Asset -> Expense, never into Revenue.
	code, body = call(t, r, "POST", "/api/v1/entries", tok, entryBody("expense", "2026-09-27", 100, bank, gaji))
	expect(t, "expense into revenue", code, 400, body)
	code, body = call(t, r, "POST", "/api/v1/entries", tok, entryBody("expense", "2026-09-27", 100, makan, bank))
	expect(t, "expense reversed (old Excel refund style)", code, 400, body)
	code, body = call(t, r, "POST", "/api/v1/entries", tok, entryBody("transfer", "2026-09-27", 100, bank, bank))
	expect(t, "transfer to same account", code, 400, body)
	code, body = call(t, r, "POST", "/api/v1/entries", tok, entryBody("income", "2026-09-27", 0, gaji, bank))
	expect(t, "zero amount", code, 400, body)
	code, body = call(t, r, "POST", "/api/v1/entries", tok, entryBody("income", "27-09-2026", 100, gaji, bank))
	expect(t, "bad date", code, 400, body)

	// Running balance in Bank: +10,826,012.92 -30,000 -5,000,000 = 5,796,012.92
	code, body = call(t, r, "GET", "/api/v1/accounts/"+bank+"/ledger", tok, "")
	expect(t, "bank ledger", code, 200, body)
	if closing := body["data"].(map[string]any)["closing"]; closing != float64(579601292) {
		t.Fatalf("bank closing balance %v, want 579601292", closing)
	}
	// Revenue balances are positive on their normal (credit) side.
	code, body = call(t, r, "GET", "/api/v1/accounts/"+gaji+"/ledger", tok, "")
	expect(t, "gaji ledger", code, 200, body)
	if closing := body["data"].(map[string]any)["closing"]; closing != float64(1082601292) {
		t.Fatalf("gaji closing balance %v, want 1082601292", closing)
	}

	code, body = call(t, r, "GET", "/api/v1/reports/trial-balance?as_of=2026-10-16", tok, "")
	expect(t, "trial balance", code, 200, body)
	tb := body["data"].(map[string]any)
	if tb["balanced"] != true || tb["total_debit"] != tb["total_credit"] || tb["total_debit"] != float64(1082601292) {
		t.Fatalf("trial balance does not balance: debit %v credit %v", tb["total_debit"], tb["total_credit"])
	}

	// Inactive accounts cannot receive new entries.
	code, body = call(t, r, "PATCH", "/api/v1/accounts/"+makan, tok, `{"is_active":false}`)
	expect(t, "deactivate", code, 200, body)
	code, body = call(t, r, "POST", "/api/v1/entries", tok, entryBody("expense", "2026-09-28", 100, bank, makan))
	expect(t, "expense to inactive account", code, 400, body)

	code, body = call(t, r, "POST", "/api/v1/accounts", tok, `{"code":"5108","name":" Kopi ","type":"Expense"}`)
	expect(t, "create account", code, 201, body)
	if a := body["data"].(map[string]any); a["name"] != "Kopi" || a["normal_balance"] != "Debit" {
		t.Fatalf("new account: %v", a)
	}
	code, body = call(t, r, "POST", "/api/v1/accounts", tok, `{"code":"5108","name":"Kopi lagi","type":"Expense"}`)
	expect(t, "duplicate account code", code, 409, body)

	code, body = call(t, r, "GET", "/api/v1/entries", tok, "")
	expect(t, "list entries", code, 200, body)
	first := body["data"].([]any)[0].(map[string]any)["id"].(string)
	code, body = call(t, r, "DELETE", "/api/v1/entries/"+first, tok, "")
	expect(t, "delete entry", code, 204, body)
	code, body = call(t, r, "GET", "/api/v1/entries", tok, "")
	if body["total"] != float64(2) {
		t.Fatalf("after delete total %v, want 2", body["total"])
	}
}

func TestLedgerIsolatesUsers(t *testing.T) {
	r, _ := newTestServer()
	lida := signUp(t, r, "lida@x.com")
	fikri := signUp(t, r, "fikri@x.com")
	lidaAcc := accountIDs(t, r, lida)
	openBooks(t, r, lida, "2026-09-01")
	openBooks(t, r, fikri, "2026-09-01")

	code, body := call(t, r, "POST", "/api/v1/entries", fikri, entryBody("income", "2026-09-25", 100, lidaAcc["4001"], lidaAcc["1001"]))
	expect(t, "post into another user's accounts", code, 404, body)
	code, body = call(t, r, "GET", "/api/v1/accounts/"+lidaAcc["1001"]+"/ledger", fikri, "")
	expect(t, "read another user's ledger", code, 404, body)

	code, body = call(t, r, "POST", "/api/v1/entries", lida, entryBody("income", "2026-09-25", 100, lidaAcc["4001"], lidaAcc["1001"]))
	expect(t, "lida posts", code, 201, body)
	id := body["data"].(map[string]any)["id"].(string)
	code, body = call(t, r, "DELETE", "/api/v1/entries/"+id, fikri, "")
	expect(t, "delete another user's entry", code, 404, body)
}

func TestOpeningAndCashCount(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "lida@x.com")
	acc := accountIDs(t, r, tok)
	bank, cash, gaji, makan := acc["1001"], acc["1002"], acc["4001"], acc["5102"]

	code, body := call(t, r, "POST", "/api/v1/entries", tok, entryBody("expense", "2026-10-01", 100, bank, makan))
	expect(t, "post before the books are open", code, 409, body)

	code, body = call(t, r, "PUT", "/api/v1/opening", tok,
		`{"start_date":"2026-10-01","items":[{"account_id":"`+acc["5102"]+`","amount":100}]}`)
	expect(t, "opening on an expense account", code, 400, body)
	code, body = call(t, r, "PUT", "/api/v1/opening", tok,
		`{"start_date":"2026-10-01","items":[{"account_id":"`+acc["1101"]+`","amount":100}]}`)
	expect(t, "opening on receivables without a contact", code, 400, body)

	// Real balances on 1 Oct: Bank 5.796.012,92 and cash 600.000.
	code, body = call(t, r, "PUT", "/api/v1/opening", tok, fmt.Sprintf(
		`{"start_date":"2026-10-01","items":[{"account_id":%q,"amount":579601292},{"account_id":%q,"amount":60000000}]}`, bank, cash))
	expect(t, "set opening", code, 200, body)
	o := body["data"].(map[string]any)
	if o["start_date"] != "2026-10-01" || o["editable"] != true {
		t.Fatalf("opening: %v", o)
	}
	code, body = call(t, r, "GET", "/api/v1/me", tok, "")
	if body["data"].(map[string]any)["start_date"] != "2026-10-01" {
		t.Fatalf("/me does not report start_date: %v", body)
	}

	code, body = call(t, r, "GET", "/api/v1/reports/trial-balance?as_of=2026-10-01", tok, "")
	tb := body["data"].(map[string]any)
	if tb["balanced"] != true || tb["total_credit"] != float64(639601292) {
		t.Fatalf("after opening, equity should be 6.396.012,92 on the credit side: %v", tb)
	}

	code, body = call(t, r, "POST", "/api/v1/entries", tok, entryBody("expense", "2026-09-30", 100, bank, makan))
	expect(t, "entry before start date", code, 400, body)
	code, body = call(t, r, "POST", "/api/v1/entries", tok, entryBody("income", "2026-10-02", 5000000, gaji, bank))
	expect(t, "entry after opening", code, 201, body)

	code, body = call(t, r, "PUT", "/api/v1/opening", tok, `{"start_date":"2026-10-01","items":[]}`)
	expect(t, "change opening once books have entries", code, 409, body)
	code, body = call(t, r, "GET", "/api/v1/entries", tok, "")
	for _, e := range body["data"].([]any) {
		if e := e.(map[string]any); e["kind"] == "opening" {
			code, body = call(t, r, "DELETE", "/api/v1/entries/"+e["id"].(string), tok, "")
			expect(t, "delete opening entry", code, 409, body)
		}
	}

	// Cash count: 540.000 in the wallet vs 600.000 in the books -> 60.000 unrecorded spending.
	code, body = call(t, r, "POST", "/api/v1/cash-counts", tok, `{"account_id":"`+cash+`","date":"2026-10-15","actual":54000000}`)
	expect(t, "cash count short", code, 201, body)
	res := body["data"].(map[string]any)
	lines := res["entry"].(map[string]any)["lines"].([]any)
	if res["diff"] != float64(-6000000) || lines[0].(map[string]any)["code"] != "1002" || lines[0].(map[string]any)["credit"] != float64(6000000) ||
		lines[1].(map[string]any)["code"] != "5199" || lines[1].(map[string]any)["debit"] != float64(6000000) {
		t.Fatalf("short cash must be Dr 5199 / Cr Cash: %v", res)
	}
	// Found 1.000 more later -> income 4199, never a negative expense.
	code, body = call(t, r, "POST", "/api/v1/cash-counts", tok, `{"account_id":"`+cash+`","date":"2026-10-16","actual":54100000}`)
	expect(t, "cash count over", code, 201, body)
	lines = body["data"].(map[string]any)["entry"].(map[string]any)["lines"].([]any)
	if lines[1].(map[string]any)["code"] != "4199" || lines[1].(map[string]any)["credit"] != float64(100000) {
		t.Fatalf("extra cash must be Cr 4199: %v", lines)
	}
	code, body = call(t, r, "POST", "/api/v1/cash-counts", tok, `{"account_id":"`+cash+`","date":"2026-10-16","actual":54100000}`)
	expect(t, "cash count equal", code, 201, body)
	if body["data"].(map[string]any)["entry"] != nil {
		t.Fatalf("no difference must post nothing: %v", body)
	}

	code, body = call(t, r, "GET", "/api/v1/accounts/"+cash+"/balance?date=2026-10-16", tok, "")
	if body["data"].(map[string]any)["balance"] != float64(54100000) {
		t.Fatalf("cash balance after counts: %v", body)
	}

	// Books: 5.796.012,92 + 50.000 salary = 5.846.012,92. Statement shows 2.500 less (admin fee).
	code, body = call(t, r, "POST", "/api/v1/reconciliations", tok, fmt.Sprintf(
		`{"account_id":%q,"date":"2026-10-16","actual":584351292,"counter_account_id":%q}`, bank, acc["5107"]))
	expect(t, "reconcile bank", code, 201, body)
	if body["data"].(map[string]any)["diff"] != float64(-250000) {
		t.Fatalf("bank diff: %v", body)
	}
	code, body = call(t, r, "POST", "/api/v1/reconciliations", tok, fmt.Sprintf(
		`{"account_id":%q,"date":"2026-10-16","actual":1,"counter_account_id":%q}`, bank, cash))
	expect(t, "reconcile against an asset", code, 400, body)
}
