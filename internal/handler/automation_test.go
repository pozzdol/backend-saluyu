package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/repository"
	"github.com/pozzdol/backend-saluyu/internal/statement"
)

// fakeAutomation keeps recurring rules and import state like the SQL tables do.
type fakeAutomation struct {
	mu       sync.Mutex
	rules    map[uuid.UUID]*model.RecurringRule
	occurs   map[uuid.UUID]map[string]bool // rule -> handled dates
	catRules map[uuid.UUID][]model.CategoryRule
	imported map[uuid.UUID]map[string]uuid.UUID // account -> hex hash -> entry
	profiles map[uuid.UUID]statement.Mapping
}

var automations sync.Map // *fakeLedger -> *fakeAutomation

func (f *fakeLedger) auto() *fakeAutomation {
	v, _ := automations.LoadOrStore(f, &fakeAutomation{rules: map[uuid.UUID]*model.RecurringRule{}, occurs: map[uuid.UUID]map[string]bool{},
		catRules: map[uuid.UUID][]model.CategoryRule{}, imported: map[uuid.UUID]map[string]uuid.UUID{}, profiles: map[uuid.UUID]statement.Mapping{}})
	return v.(*fakeAutomation)
}

func (f *fakeLedger) SaveRule(_ context.Context, r *model.RecurringRule) error {
	a := f.auto()
	a.mu.Lock()
	defer a.mu.Unlock()
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	} else if old, ok := a.rules[r.ID]; !ok || old.UserID != r.UserID {
		return repository.ErrRuleNotFound
	}
	copied := *r
	a.rules[r.ID] = &copied
	return nil
}

func (f *fakeLedger) DeleteRule(_ context.Context, userID, id uuid.UUID) error {
	a := f.auto()
	a.mu.Lock()
	defer a.mu.Unlock()
	if r, ok := a.rules[id]; !ok || r.UserID != userID {
		return repository.ErrRuleNotFound
	}
	delete(a.rules, id)
	return nil
}

func (f *fakeLedger) Rules(_ context.Context, userID uuid.UUID) ([]model.RecurringRule, error) {
	return f.auto().list(func(r *model.RecurringRule) bool { return r.UserID == userID }), nil
}

func (a *fakeAutomation) list(keep func(*model.RecurringRule) bool) []model.RecurringRule {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []model.RecurringRule{}
	for _, r := range a.rules {
		if keep(r) {
			out = append(out, *r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (f *fakeLedger) DueRules(_ context.Context, today time.Time, userID *uuid.UUID, mode string) ([]model.RecurringRule, error) {
	day := today.Format(time.DateOnly)
	return f.auto().list(func(r *model.RecurringRule) bool {
		return r.NextDate <= day && (r.EndDate == nil || r.NextDate <= *r.EndDate) &&
			(userID == nil || r.UserID == *userID) && (mode == "" || r.Mode == mode)
	}), nil
}

func (f *fakeLedger) Handled(_ context.Context, ruleID uuid.UUID, from time.Time) (map[string]bool, error) {
	a := f.auto()
	a.mu.Lock()
	defer a.mu.Unlock()
	out := map[string]bool{}
	for d := range a.occurs[ruleID] {
		if d >= from.Format(time.DateOnly) {
			out[d] = true
		}
	}
	return out, nil
}

func (f *fakeLedger) HandleOccurrence(_ context.Context, userID, ruleID uuid.UUID, due time.Time, e *model.Entry, next time.Time) error {
	a := f.auto()
	a.mu.Lock()
	defer a.mu.Unlock()
	r, ok := a.rules[ruleID]
	if !ok || r.UserID != userID {
		return repository.ErrRuleNotFound
	}
	day := due.Format(time.DateOnly)
	if a.occurs[ruleID][day] {
		return repository.ErrOccurrenceHandled
	}
	if a.occurs[ruleID] == nil {
		a.occurs[ruleID] = map[string]bool{}
	}
	a.occurs[ruleID][day] = true
	if e != nil {
		f.mu.Lock()
		f.insertEntry(userID, e)
		f.mu.Unlock()
	}
	r.NextDate = max(r.NextDate, next.Format(time.DateOnly))
	r.LastError = nil
	return nil
}

func (f *fakeLedger) SetRuleError(_ context.Context, ruleID uuid.UUID, msg string) error {
	a := f.auto()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rules[ruleID].LastError = &msg
	return nil
}

func (f *fakeLedger) CategoryRules(_ context.Context, userID uuid.UUID) ([]model.CategoryRule, error) {
	a := f.auto()
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]model.CategoryRule{}, a.catRules[userID]...), nil
}

func (f *fakeLedger) CreateCategoryRule(_ context.Context, userID uuid.UUID, c *model.CategoryRule) error {
	a := f.auto()
	a.mu.Lock()
	defer a.mu.Unlock()
	c.ID = uuid.New()
	a.catRules[userID] = append(a.catRules[userID], *c)
	return nil
}

func (f *fakeLedger) DeleteCategoryRule(_ context.Context, userID, id uuid.UUID) error {
	a := f.auto()
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, c := range a.catRules[userID] {
		if c.ID == id {
			a.catRules[userID] = append(a.catRules[userID][:i], a.catRules[userID][i+1:]...)
			return nil
		}
	}
	return repository.ErrRuleMissing
}

func (f *fakeLedger) ImportProfile(_ context.Context, accountID uuid.UUID) (*statement.Mapping, error) {
	a := f.auto()
	a.mu.Lock()
	defer a.mu.Unlock()
	if m, ok := a.profiles[accountID]; ok {
		return &m, nil
	}
	return nil, nil
}

func (f *fakeLedger) ImportedHashes(_ context.Context, accountID uuid.UUID, hashes []string) (map[string]uuid.UUID, error) {
	a := f.auto()
	a.mu.Lock()
	defer a.mu.Unlock()
	out := map[string]uuid.UUID{}
	for _, h := range hashes {
		if id, ok := a.imported[accountID][h]; ok {
			out[h] = id
		}
	}
	return out, nil
}

func (f *fakeLedger) MatchCandidates(_ context.Context, userID, accountID uuid.UUID, from, to time.Time) ([]model.MatchCandidate, error) {
	a := f.auto()
	a.mu.Lock()
	linked := map[uuid.UUID]bool{}
	for _, id := range a.imported[accountID] {
		linked[id] = true
	}
	a.mu.Unlock()
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.MatchCandidate{}
	for _, e := range f.entries[userID] {
		if linked[e.ID] || e.EntryDate.Before(from) || e.EntryDate.After(to) {
			continue
		}
		var sum int64
		touches := false
		for _, l := range e.Lines {
			if l.AccountID == accountID {
				sum += l.Debit - l.Credit
				touches = true
			}
		}
		if touches {
			out = append(out, model.MatchCandidate{EntryID: e.ID, EntryNo: e.EntryNo, Date: e.EntryDate.Format(time.DateOnly), Amount: sum})
		}
	}
	return out, nil
}

func (f *fakeLedger) CommitImport(_ context.Context, userID, accountID uuid.UUID, rows []model.ImportedRow, mapping statement.Mapping) (int, int, int, error) {
	a := f.auto()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.imported[accountID] == nil {
		a.imported[accountID] = map[string]uuid.UUID{}
	}
	var posted, matched, skipped int
	for _, r := range rows {
		h := hexString(r.Hash)
		if _, ok := a.imported[accountID][h]; ok {
			skipped++
			continue
		}
		if r.Entry != nil {
			f.mu.Lock()
			f.insertEntry(userID, r.Entry)
			f.mu.Unlock()
			a.imported[accountID][h] = r.Entry.ID
			posted++
		} else {
			a.imported[accountID][h] = *r.EntryID
			matched++
		}
	}
	a.profiles[accountID] = mapping
	return posted, matched, skipped, nil
}

func hexString(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, c := range b {
		out[i*2], out[i*2+1] = digits[c>>4], digits[c&15]
	}
	return string(out)
}

func TestRecurring(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "rec@x.com")
	now := time.Now()
	day := func(n int) string { return now.AddDate(0, 0, n).Format(time.DateOnly) }
	openBooks(t, r, tok, day(-10))
	_, body := call(t, r, "GET", "/api/v1/accounts", tok, "")
	accounts := body["data"].([]any)
	bank, makan, gaji := accountByCode(t, accounts, "1001"), accountByCode(t, accounts, "5102"), accountByCode(t, accounts, "4001")

	code, body := call(t, r, "POST", "/api/v1/recurring", tok, `{"name":"Kopi","kind":"expense","from_account_id":"`+bank+`","to_account_id":"`+makan+`","amount":2000000,"frequency":"daily","mode":"remind","start_date":"`+day(-2)+`"}`)
	expect(t, "create reminder", code, 201, body)
	kopi := body["data"].(map[string]any)["id"].(string)
	code, body = call(t, r, "POST", "/api/v1/recurring", tok, `{"name":"Gaji","kind":"income","from_account_id":"`+gaji+`","to_account_id":"`+bank+`","amount":1000000000,"frequency":"daily","mode":"auto","start_date":"`+day(-1)+`"}`)
	expect(t, "create auto", code, 201, body)
	code, body = call(t, r, "POST", "/api/v1/recurring", tok, `{"name":"Sewa","kind":"expense","from_account_id":"`+bank+`","to_account_id":"`+makan+`","amount":1,"frequency":"weekly","mode":"remind","start_date":"`+day(0)+`"}`)
	if code != 400 || body["code"] != "needs_day" {
		t.Fatalf("weekly without a weekday: %d %v", code, body)
	}

	// The auto rule posts yesterday and today; asking twice posts nothing more.
	countKind := func(desc string) int {
		_, body := call(t, r, "GET", "/api/v1/entries?limit=100", tok, "")
		n := 0
		for _, e := range body["data"].([]any) {
			if e.(map[string]any)["description"] == desc {
				n++
			}
		}
		return n
	}
	_, body = call(t, r, "GET", "/api/v1/recurring/due", tok, "")
	call(t, r, "GET", "/api/v1/recurring/due", tok, "")
	if n := countKind("Gaji"); n != 2 {
		t.Fatalf("auto rule posted %d entries, want 2", n)
	}
	due := body["data"].([]any)
	if len(due) != 3 { // Kopi: 2 days ago, yesterday, today
		t.Fatalf("due: %v", due)
	}

	confirm := func(date, extra string) (int, map[string]any) {
		return call(t, r, "POST", "/api/v1/recurring/"+kopi+"/confirm", tok, `{"due_date":"`+date+`"`+extra+`}`)
	}
	code, body = confirm(day(-1), `,"amount":2500000`)
	expect(t, "confirm yesterday with another amount", code, 204, body)
	code, body = confirm(day(-1), "")
	expect(t, "confirm the same day again", code, 409, body)
	code, body = call(t, r, "POST", "/api/v1/recurring/"+kopi+"/skip", tok, `{"due_date":"`+day(-2)+`"}`)
	expect(t, "skip", code, 204, body)
	code, body = confirm(day(3), "")
	expect(t, "confirm a future date", code, 409, body)

	_, body = call(t, r, "GET", "/api/v1/recurring/due", tok, "")
	if due := body["data"].([]any); len(due) != 1 || due[0].(map[string]any)["due_date"] != day(0) {
		t.Fatalf("only today should wait: %v", due)
	}
	if n := countKind("Kopi"); n != 1 {
		t.Fatalf("kopi entries %d", n)
	}
}

func upload(t *testing.T, r *gin.Engine, tok, account, name, content string) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", name)
	fw.Write([]byte(content))
	w.WriteField("account_id", account)
	w.Close()
	req := httptest.NewRequest("POST", "/api/v1/imports/preview", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestImportStatement(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "imp@x.com")
	now := time.Now()
	d := func(n int) string { return now.AddDate(0, 0, n).Format("02/01/2006") }
	iso := func(n int) string { return now.AddDate(0, 0, n).Format(time.DateOnly) }
	openBooks(t, r, tok, iso(-30))
	_, body := call(t, r, "GET", "/api/v1/accounts", tok, "")
	accounts := body["data"].([]any)
	bank, transport := accountByCode(t, accounts, "1001"), accountByCode(t, accounts, "5101")

	// Recorded by hand before the import: it should be matched, not duplicated.
	call(t, r, "POST", "/api/v1/entries", tok, `{"kind":"expense","entry_date":"`+iso(-4)+`","amount":4500000,"from_account_id":"`+bank+`","to_account_id":"`+transport+`"}`)
	code, body := call(t, r, "POST", "/api/v1/category-rules", tok, `{"keyword":"grab","account_id":"`+transport+`"}`)
	expect(t, "category rule", code, 201, body)

	csv := "Tanggal;Keterangan;Mutasi;Saldo\n" +
		d(-5) + ";GRAB* RIDE;45.000,00 DB;1.000.000,00\n" +
		d(-3) + ";KOPI;20.000,00 DB;980.000,00\n" +
		d(-3) + ";KOPI;20.000,00 DB;960.000,00\n" +
		d(-2) + ";GRAB* FOOD;55.000,00 DB;905.000,00\n" +
		d(-1) + ";BUNGA;1.000,00 CR;906.000,00\n"
	code, body = upload(t, r, tok, bank, "bca.csv", csv)
	expect(t, "preview", code, 200, body)
	rows := body["data"].(map[string]any)["rows"].([]any)
	if len(rows) != 5 {
		t.Fatalf("rows %v", rows)
	}
	status := func(i int) map[string]any { return rows[i].(map[string]any) }
	if status(0)["status"] != "matched" || status(1)["status"] != "new" || status(1)["hash"] == status(2)["hash"] {
		t.Fatalf("matched by hand entry, two equal coffees stay two rows: %v %v %v", status(0), status(1), status(2))
	}
	if status(3)["suggested_account_id"] != transport {
		t.Fatalf("grab rule: %v", status(3))
	}

	commit := func(rows []any) (int, map[string]any) {
		var list []map[string]any
		for _, x := range rows {
			x := x.(map[string]any)
			row := map[string]any{"hash": x["hash"], "date": x["date"], "description": x["description"], "amount": x["amount"]}
			if x["status"] == "matched" {
				row["match_entry_id"] = x["entry_id"]
			}
			if x["suggested_account_id"] != nil {
				row["account_id"] = x["suggested_account_id"]
			}
			list = append(list, row)
		}
		b, _ := json.Marshal(map[string]any{"account_id": bank, "mapping": body["data"].(map[string]any)["mapping"], "rows": list})
		return call(t, r, "POST", "/api/v1/imports/commit", tok, string(b))
	}
	code, res := commit(rows)
	expect(t, "commit", code, 201, res)
	if got := res["data"].(map[string]any); got["posted"].(float64) != 4 || got["matched"].(float64) != 1 {
		t.Fatalf("result %v", got)
	}

	// The same file again: every row is a duplicate, and committing posts nothing.
	_, body = upload(t, r, tok, bank, "bca.csv", csv)
	rows = body["data"].(map[string]any)["rows"].([]any)
	for _, x := range rows {
		if x.(map[string]any)["status"] != "duplicate" {
			t.Fatalf("re-import should be all duplicates: %v", x)
		}
	}
	code, res = commit(rows)
	expect(t, "commit again", code, 201, res)
	if got := res["data"].(map[string]any); got["posted"].(float64) != 0 || got["skipped"].(float64) != 5 {
		t.Fatalf("second commit %v", got)
	}
	_, body = call(t, r, "GET", "/api/v1/entries?limit=100", tok, "")
	if n := len(body["data"].([]any)); n != 5 { // the hand entry + 4 imported (a zero opening has no entry)
		t.Fatalf("entries %d", n)
	}
}
