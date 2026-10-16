package handler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/repository"
)

// fakeExtras holds goals and invite links like their SQL tables.
type fakeExtras struct {
	mu    sync.Mutex
	goals map[uuid.UUID]model.Goal
	links map[string]*fakeLink11
}

type fakeLink11 struct {
	group   uuid.UUID
	expires time.Time
	used    bool
}

var extras sync.Map // *fakeLedger -> *fakeExtras

func (f *fakeLedger) extra() *fakeExtras {
	v, _ := extras.LoadOrStore(f, &fakeExtras{goals: map[uuid.UUID]model.Goal{}, links: map[string]*fakeLink11{}})
	return v.(*fakeExtras)
}

func (f *fakeLedger) Goals(_ context.Context, userID uuid.UUID) ([]model.Goal, error) {
	x := f.extra()
	x.mu.Lock()
	defer x.mu.Unlock()
	out := []model.Goal{}
	for _, g := range x.goals {
		if g.UserID == userID {
			out = append(out, g)
		}
	}
	return out, nil
}

func (f *fakeLedger) SaveGoal(_ context.Context, g *model.Goal) error {
	x := f.extra()
	x.mu.Lock()
	defer x.mu.Unlock()
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	} else if old, ok := x.goals[g.ID]; !ok || old.UserID != g.UserID {
		return repository.ErrGoalNotFound
	}
	x.goals[g.ID] = *g
	return nil
}

func (f *fakeLedger) DeleteGoal(_ context.Context, userID, id uuid.UUID) error {
	x := f.extra()
	x.mu.Lock()
	defer x.mu.Unlock()
	if g, ok := x.goals[id]; !ok || g.UserID != userID {
		return repository.ErrGoalNotFound
	}
	delete(x.goals, id)
	return nil
}

func (f *fakeLedger) CreateInviteLink(_ context.Context, groupID, _ uuid.UUID, hash []byte, expires time.Time) error {
	x := f.extra()
	x.mu.Lock()
	defer x.mu.Unlock()
	x.links[string(hash)] = &fakeLink11{group: groupID, expires: expires}
	return nil
}

func (f *fakeLedger) AcceptInviteLink(_ context.Context, hash []byte, userID uuid.UUID) (uuid.UUID, error) {
	x := f.extra()
	x.mu.Lock()
	defer x.mu.Unlock()
	l, ok := x.links[string(hash)]
	if !ok || l.used || time.Now().After(l.expires) {
		return uuid.Nil, repository.ErrInvalidInvite
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	g := f.share.groups[l.group]
	if g.members[userID] == "active" {
		return uuid.Nil, repository.ErrAlreadyInGroup
	}
	g.members[userID] = "active"
	l.used = true
	return l.group, nil
}

func TestClosedBooks(t *testing.T) {
	b := newBook(t, "lock@x.com", "2026-01-01", "1001=1000000000")
	b.post("expense", "2026-02-10", 1000000, "1001", "5102")
	_, body := b.r("GET", "/api/v1/entries?kind=expense", "")
	feb := body["data"].([]any)[0].(map[string]any)["id"].(string)

	code, body := b.r("PUT", "/api/v1/me/lock", `{"locked_until":"2999-01-01"}`)
	if code != 400 || body["code"] != "lock_in_future" {
		t.Fatalf("closing days that have not happened: want 400 lock_in_future, got %d %v", code, body)
	}
	code, body = b.r("PUT", "/api/v1/me/lock", `{"locked_until":"2026-02-24"}`)
	expect(t, "lock", code, 200, body)
	if body["data"].(map[string]any)["locked_until"] != "2026-02-24" {
		t.Fatalf("user carries the lock: %v", body)
	}

	locked := func(step string, code int, body map[string]any) {
		t.Helper()
		if code != 409 || body["code"] != "period_locked" {
			t.Fatalf("%s: want 409 period_locked, got %d %v", step, code, body)
		}
	}
	code, body = b.r("POST", "/api/v1/entries", `{"kind":"expense","entry_date":"2026-02-20","amount":100,"from_account_id":"`+b.acc["1001"]+`","to_account_id":"`+b.acc["5102"]+`"}`)
	locked("post into closed books", code, body)
	code, body = b.r("PUT", "/api/v1/entries/"+feb, `{"kind":"expense","entry_date":"2026-03-01","amount":100,"from_account_id":"`+b.acc["1001"]+`","to_account_id":"`+b.acc["5102"]+`"}`)
	locked("move an entry out of closed books", code, body)
	code, body = b.r("DELETE", "/api/v1/entries/"+feb, "")
	locked("delete in closed books", code, body)
	code, body = b.r("PUT", "/api/v1/opening", `{"start_date":"2026-01-01","items":[]}`)
	locked("change the opening in closed books", code, body)

	// The fix for a closed mistake: a reversing entry dated in the open period.
	code, body = b.r("POST", "/api/v1/entries/"+feb+"/reverse", `{"entry_date":"2026-03-01"}`)
	expect(t, "reverse", code, 201, body)
	lines := body["data"].(map[string]any)["lines"].([]any)
	if num(lines[0].(map[string]any)["credit"]) != 1000000 || body["data"].(map[string]any)["kind"] != "adjustment" {
		t.Fatalf("reversed lines: %v", lines)
	}
	code, body = b.r("POST", "/api/v1/entries/journal", `{"entry_date":"2026-03-02","description":"koreksi","lines":[
		{"account_id":"`+b.acc["5103"]+`","debit":500000,"credit":0},{"account_id":"`+b.acc["1001"]+`","debit":0,"credit":500000}]}`)
	expect(t, "manual journal", code, 201, body)
	code, body = b.r("POST", "/api/v1/entries/journal", `{"entry_date":"2026-03-02","lines":[
		{"account_id":"`+b.acc["5103"]+`","debit":500000,"credit":0},{"account_id":"`+b.acc["1001"]+`","debit":0,"credit":400000}]}`)
	if code != 400 || body["code"] != "unbalanced" {
		t.Fatalf("unbalanced journal: %d %v", code, body)
	}

	code, body = b.r("PUT", "/api/v1/me/lock", `{"locked_until":null}`)
	expect(t, "unlock", code, 200, body)
	code, body = b.r("DELETE", "/api/v1/entries/"+feb, "")
	expect(t, "delete after unlocking", code, 204, body)
}

func TestClientIDPostsOnce(t *testing.T) {
	b := newBook(t, "cid@x.com", "2026-01-01", "1001=1000000000")
	id := uuid.NewString()
	body := `{"kind":"expense","entry_date":"2026-02-10","amount":2500000,"from_account_id":"` + b.acc["1001"] + `","to_account_id":"` + b.acc["5102"] + `","client_id":"` + id + `"}`
	_, first := b.r("POST", "/api/v1/entries", body)
	_, second := b.r("POST", "/api/v1/entries", body)
	if first["data"].(map[string]any)["id"] != second["data"].(map[string]any)["id"] {
		t.Fatalf("the retry must return the first entry: %v %v", first, second)
	}
	_, list := b.r("GET", "/api/v1/entries?limit=10", "")
	if n := len(list["data"].([]any)); n != 2 { // opening + one expense
		t.Fatalf("entries %d", n)
	}
}

func TestGoalsAndInviteLinks(t *testing.T) {
	r, _ := newTestServer()
	lida := signUp(t, r, "lida@x.com")
	call(t, r, "PUT", "/api/v1/opening", lida, `{"start_date":"2026-01-01","items":[]}`)
	_, body := call(t, r, "GET", "/api/v1/accounts", lida, "")
	accounts := body["data"].([]any)
	tabungan, makan := accountByCode(t, accounts, "1003"), accountByCode(t, accounts, "5102")
	code, body := call(t, r, "POST", "/api/v1/goals", lida, `{"name":"Liburan","target":500000000,"account_id":"`+makan+`"}`)
	expect(t, "goal on an expense account", code, 400, body)
	deadline := time.Now().AddDate(0, 5, 0).Format(time.DateOnly)
	code, body = call(t, r, "POST", "/api/v1/goals", lida, `{"name":"Liburan","target":500000000,"deadline":"`+deadline+`","account_id":"`+tabungan+`"}`)
	expect(t, "goal", code, 201, body)
	if g := body["data"].(map[string]any); num(g["monthly_needed"]) != 100000000 || num(g["months_left"]) != 5 {
		t.Fatalf("Rp5 juta in 5 months: %v", g)
	}

	_, body = call(t, r, "POST", "/api/v1/groups", lida, `{"name":"Rumah"}`)
	gid := body["data"].(map[string]any)["id"].(string)
	code, body = call(t, r, "POST", "/api/v1/groups/"+gid+"/invite-links", lida, "")
	expect(t, "invite link", code, 201, body)
	tok := body["data"].(map[string]any)["token"].(string)

	accept := func(who string) (int, map[string]any) {
		return call(t, r, "POST", "/api/v1/invite-links/accept", who, `{"token":"`+tok+`"}`)
	}
	code, body = accept(lida)
	if code != 409 || body["code"] != "already_in_group" {
		t.Fatalf("the owner's own link: %d %v", code, body)
	}
	// An unverified account may join by link: the link is the proof.
	call(t, r, "POST", "/api/v1/register", "", `{"name":"Fikri","email":"fikri@x.com","password":"secret123"}`)
	_, body = call(t, r, "POST", "/api/v1/login", "", `{"email":"fikri@x.com","password":"secret123"}`)
	fikri := body["access_token"].(string)
	code, body = accept(fikri)
	expect(t, "join by link", code, 200, body)
	budi := signUp(t, r, "budi@x.com")
	code, body = accept(budi)
	if code != 400 || body["code"] != "invalid_invite" {
		t.Fatalf("a used link: %d %v", code, body)
	}
	_, body = call(t, r, "GET", "/api/v1/groups", fikri, "")
	if n := len(body["data"].([]any)); n != 1 {
		t.Fatalf("fikri is in %d groups", n)
	}
}
