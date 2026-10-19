package handler

import (
	"context"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/service"
	"github.com/pozzdol/backend-saluyu/internal/token"
)

// EntryDays: days with an entry the user recorded, newest first.
func (f *fakeLedger) EntryDays(_ context.Context, userID uuid.UUID, from time.Time) ([]time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	seen := map[time.Time]bool{}
	var out []time.Time
	for _, e := range f.entries[userID] {
		if e.EntryDate.Before(from) || e.Kind == "opening" || e.Kind == "cash_count" || e.Kind == "adjustment" || seen[e.EntryDate] {
			continue
		}
		seen[e.EntryDate] = true
		out = append(out, e.EntryDate)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].After(out[j]) })
	return out, nil
}

// fakeNotify is an in-memory service.NotifyRepository; recipients are the verified users of fakeRepo.
type fakeNotify struct {
	mu     sync.Mutex
	users  *fakeRepo
	prefs  map[uuid.UUID]model.NotifyPrefs
	claims map[string]bool
}

func (f *fakeNotify) Prefs(_ context.Context, id uuid.UUID) (model.NotifyPrefs, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p, ok := f.prefs[id]; ok {
		return p, nil
	}
	return model.NotifyPrefs{DueBills: true, Budget: true, Weekly: true, Inactive: true}, nil
}

func (f *fakeNotify) SetPrefs(_ context.Context, id uuid.UUID, p model.NotifyPrefs) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prefs[id] = p
	return nil
}

func (f *fakeNotify) Recipients(ctx context.Context) ([]model.NotifyUser, error) {
	f.users.mu.Lock()
	var out []model.NotifyUser
	for _, u := range f.users.users {
		if u.IsActive && u.EmailVerifiedAt != nil {
			out = append(out, model.NotifyUser{ID: u.ID, Name: u.Name, Email: u.Email, Locale: u.Locale})
		}
	}
	f.users.mu.Unlock()
	for i := range out {
		out[i].Prefs, _ = f.Prefs(ctx, out[i].ID)
	}
	return out, nil
}

func (f *fakeNotify) Claim(_ context.Context, id uuid.UUID, kind, ref string, _ time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := id.String() + kind + ref
	if f.claims[k] {
		return false, nil
	}
	f.claims[k] = true
	return true, nil
}

func (f *fakeNotify) Unclaim(_ context.Context, id uuid.UUID, kind, ref string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.claims, id.String()+kind+ref)
	return nil
}

func TestEmailReminders(t *testing.T) {
	r, repo := newTestServer()
	fl := repo.ledger
	box := &fakeMail{}
	ledgerSvc := service.NewLedgerService(fl)
	svc := service.NewNotifyService(&fakeNotify{users: repo, prefs: map[uuid.UUID]model.NotifyPrefs{}, claims: map[string]bool{}},
		service.NewReportService(fl), service.NewRecurringService(fl, ledgerSvc), service.NewCreditService(fl, ledgerSvc), box, "http://app.test")
	RegisterNotifyRoutes(r, NewNotifyHandler(svc), token.NewMaker("test-secret", time.Minute))

	tok := signUp(t, r, "n@x.com")
	openBooks(t, r, tok, "2026-02-01")
	ids := accountIDs(t, r, tok)
	post := func(kind, date string, amount int64, from, to string) {
		t.Helper()
		code, body := call(t, r, "POST", "/api/v1/entries", tok, entryBody(kind, date, amount, ids[from], ids[to]))
		expect(t, kind+" "+date, code, 201, body)
	}
	post("income", "2026-02-20", 100000000, "4001", "1001")
	post("expense", "2026-02-27", 1000000, "1001", "5102")
	post("expense", "2026-02-28", 1000000, "1001", "5102")
	post("expense", "2026-03-01", 7000000, "1001", "5102") // 90% of the budget below: crosses 80%
	code, body := call(t, r, "PUT", "/api/v1/budgets/"+ids["5102"], tok, `{"date":"2026-03-01","amount":10000000}`)
	expect(t, "budget", code, 200, body)
	code, body = call(t, r, "POST", "/api/v1/recurring", tok, `{"name":"Sewa","kind":"expense","from_account_id":"`+ids["1001"]+`","to_account_id":"`+ids["5102"]+
		`","amount":3000000,"frequency":"monthly","day":5,"mode":"remind","start_date":"2026-03-01"}`)
	expect(t, "rule", code, 201, body)

	// Streak: 27 Feb – 1 Mar, ending yesterday; 26 Feb is empty and the opening day does not count.
	code, body = call(t, r, "GET", "/api/v1/dashboard?date=2026-03-02", tok, "")
	expect(t, "dashboard", code, 200, body)
	if n := num(body["data"].(map[string]any)["streak_days"]); n != 3 {
		t.Fatalf("streak_days %d, want 3", n)
	}

	subjects := func() []string {
		box.mu.Lock()
		defer box.mu.Unlock()
		var out []string
		for _, m := range box.sent {
			out = append(out, m.Subject)
		}
		sort.Strings(out)
		return out
	}
	run := func(day string, hour int) {
		t.Helper()
		d, _ := time.Parse(time.DateOnly, day)
		if err := svc.Run(context.Background(), d.Add(time.Duration(hour)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	want := func(step string, subjects_ ...string) {
		t.Helper()
		sort.Strings(subjects_)
		if got := subjects(); strings.Join(got, "|") != strings.Join(subjects_, "|") {
			t.Fatalf("%s: sent %q, want %q", step, got, subjects_)
		}
	}

	// Monday 2 March: the rent is due in 3 days, the budget is past 80%, and it is weekly day.
	run("2026-03-02", 3)
	want("before 07:00")
	run("2026-03-02", 8)
	want("monday", "Tagihan segera jatuh tempo", "Anggaranmu hampir habis", "Ringkasan mingguanmu")
	run("2026-03-02", 9)
	want("monday again", "Tagihan segera jatuh tempo", "Anggaranmu hampir habis", "Ringkasan mingguanmu")

	// Wednesday 4 March: three days since the last entry, sent once.
	run("2026-03-04", 8)
	run("2026-03-04", 20)
	want("wednesday", "Tagihan segera jatuh tempo", "Anggaranmu hampir habis", "Ringkasan mingguanmu", "Sudah 3 hari belum mencatat")

	// Thursday 5 March, the rent day: nothing is sent while bills are off, then it is.
	code, body = call(t, r, "PUT", "/api/v1/me/notifications", tok, `{"due_bills":false,"budget":true,"weekly":true,"inactive":true}`)
	expect(t, "turn off bills", code, 200, body)
	run("2026-03-05", 8)
	want("bills off", "Tagihan segera jatuh tempo", "Anggaranmu hampir habis", "Ringkasan mingguanmu", "Sudah 3 hari belum mencatat")
	code, body = call(t, r, "PUT", "/api/v1/me/notifications", tok, `{"due_bills":true,"budget":true,"weekly":true,"inactive":true}`)
	expect(t, "turn on bills", code, 200, body)
	run("2026-03-05", 9)
	want("bills on", "Tagihan segera jatuh tempo", "Tagihan segera jatuh tempo", "Anggaranmu hampir habis", "Ringkasan mingguanmu", "Sudah 3 hari belum mencatat")

	code, body = call(t, r, "GET", "/api/v1/me/notifications", tok, "")
	expect(t, "get prefs", code, 200, body)
	if body["data"].(map[string]any)["due_bills"] != true {
		t.Fatalf("prefs %v", body)
	}
	code, body = call(t, r, "PUT", "/api/v1/me/notifications", tok, `{"due_bills":false}`)
	expect(t, "partial body", code, 400, body)
}
