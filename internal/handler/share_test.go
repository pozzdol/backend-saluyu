package handler

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/repository"
)

// fakeShare is the phase 5 state of fakeLedger. Access rules live in the service; the fake only
// mirrors what the SQL guarantees (unique invites per email, one request per entry, locked rows).
type fakeShare struct {
	users    *fakeRepo
	groups   map[uuid.UUID]*fakeGroup
	requests map[uuid.UUID]*fakeRequest
}

type fakeGroup struct {
	name    string
	created time.Time
	members map[uuid.UUID]string // user -> "active" | "left"
	invites []model.PendingInvite
	inviter map[uuid.UUID]uuid.UUID // invite id -> inviter
}

type fakeRequest struct {
	fromEntry, toEntry uuid.UUID
	from, to           uuid.UUID
	amount             int64
	status             string
	created            time.Time
}

func newFakeShare() *fakeShare {
	return &fakeShare{groups: map[uuid.UUID]*fakeGroup{}, requests: map[uuid.UUID]*fakeRequest{}}
}

func (s *fakeShare) user(id uuid.UUID) model.User {
	s.users.mu.Lock()
	defer s.users.mu.Unlock()
	return *s.users.users[id]
}

func (f *fakeLedger) shares(a, b uuid.UUID) bool {
	for _, g := range f.share.groups {
		if g.members[a] == "active" && g.members[b] == "active" {
			return true
		}
	}
	return false
}

func (f *fakeLedger) SharesActiveGroup(_ context.Context, a, b uuid.UUID) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.shares(a, b), nil
}

func (f *fakeLedger) CreateGroup(_ context.Context, userID uuid.UUID, name string) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := uuid.New()
	f.share.groups[id] = &fakeGroup{name: name, created: time.Now(), members: map[uuid.UUID]string{userID: "active"},
		inviter: map[uuid.UUID]uuid.UUID{}}
	return id, nil
}

func (f *fakeLedger) ListGroups(_ context.Context, userID uuid.UUID) ([]model.Group, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.Group{}
	for id, g := range f.share.groups {
		if g.members[userID] != "active" {
			continue
		}
		grp := model.Group{ID: id, Name: g.name, Members: f.activeMembers(g), PendingInvites: append([]model.PendingInvite{}, g.invites...)}
		out = append(out, grp)
	}
	return out, nil
}

func (f *fakeLedger) activeMembers(g *fakeGroup) []model.Member {
	out := []model.Member{}
	for uid, st := range g.members {
		if st == "active" {
			u := f.share.user(uid)
			out = append(out, model.Member{ID: uid, Name: u.Name, Email: u.Email})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (f *fakeLedger) IsActiveMember(_ context.Context, groupID, userID uuid.UUID) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	g, ok := f.share.groups[groupID]
	return ok && g.members[userID] == "active", nil
}

func (f *fakeLedger) ActiveMembers(_ context.Context, groupID uuid.UUID) ([]model.Member, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.activeMembers(f.share.groups[groupID]), nil
}

func (f *fakeLedger) Invite(_ context.Context, groupID, inviter uuid.UUID, email string) (*model.PendingInvite, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	g := f.share.groups[groupID]
	for _, inv := range g.invites {
		if inv.Email == email {
			copied := inv
			return &copied, nil
		}
	}
	inv := model.PendingInvite{ID: uuid.New(), Email: email, CreatedAt: time.Now()}
	g.invites = append(g.invites, inv)
	g.inviter[inv.ID] = inviter
	return &inv, nil
}

func (f *fakeLedger) CancelInvite(_ context.Context, groupID, inviteID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	g := f.share.groups[groupID]
	for i, inv := range g.invites {
		if inv.ID == inviteID {
			g.invites = append(g.invites[:i], g.invites[i+1:]...)
			return nil
		}
	}
	return repository.ErrNotFound
}

func (f *fakeLedger) InvitesForEmail(_ context.Context, email string) ([]model.InboxInvite, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.InboxInvite{}
	for gid, g := range f.share.groups {
		for _, inv := range g.invites {
			if inv.Email == email {
				out = append(out, model.InboxInvite{ID: inv.ID, GroupID: gid, GroupName: g.name,
					InvitedByName: f.share.user(g.inviter[inv.ID]).Name, CreatedAt: inv.CreatedAt})
			}
		}
	}
	return out, nil
}

func (f *fakeLedger) RespondInvite(_ context.Context, inviteID, userID uuid.UUID, email string, accept bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, g := range f.share.groups {
		for i, inv := range g.invites {
			if inv.ID == inviteID && inv.Email == email {
				g.invites = append(g.invites[:i], g.invites[i+1:]...)
				if accept {
					g.members[userID] = "active"
				}
				return nil
			}
		}
	}
	return repository.ErrNotFound
}

func (f *fakeLedger) LeaveGroup(_ context.Context, groupID, userID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	g, ok := f.share.groups[groupID]
	if !ok || g.members[userID] != "active" {
		return repository.ErrNotFound
	}
	g.members[userID] = "left"
	return nil
}

func (f *fakeLedger) LinkContact(_ context.Context, userID, contactID uuid.UUID, linked *uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.contacts[userID][contactID]; !ok {
		return repository.ErrContactNotFound
	}
	if linked == nil {
		delete(f.links, contactID)
		return nil
	}
	for other := range f.contacts[userID] {
		if other != contactID && f.links[other] == *linked {
			return repository.ErrAlreadyLinked
		}
	}
	f.links[contactID] = *linked
	return nil
}

// createLoanRequest mirrors the SQL in repository.createLoanRequest. Caller holds f.mu.
func (f *fakeLedger) createLoanRequest(userID uuid.UUID, e *model.Entry) {
	if e.Kind != "loan" && e.Kind != "repayment" {
		return
	}
	for _, q := range f.share.requests {
		if q.fromEntry == e.ID || q.toEntry == e.ID {
			return // a mirror never asks back, and an entry keeps its one approved request
		}
	}
	var to uuid.UUID
	var net int64
	for _, l := range e.Lines {
		if l.ContactID == nil {
			continue
		}
		if linked, ok := f.links[*l.ContactID]; ok {
			to = linked
			net += l.Debit - l.Credit
		}
	}
	if to == uuid.Nil || net == 0 {
		return
	}
	f.share.requests[uuid.New()] = &fakeRequest{fromEntry: e.ID, from: userID, to: to, amount: max(net, -net),
		status: "pending", created: time.Now()}
}

func (f *fakeLedger) entryByID(userID, id uuid.UUID) *model.Entry {
	for _, e := range f.entries[userID] {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// contactLinesTo returns the entry's lines on contacts linked to user. Caller holds f.mu.
func (f *fakeLedger) contactLinesTo(owner uuid.UUID, e *model.Entry, user uuid.UUID) []model.Line {
	var out []model.Line
	for _, l := range e.Lines {
		if l.ContactID != nil && f.links[*l.ContactID] == user {
			l.Code = f.userAccounts(owner)[l.AccountID].Code
			out = append(out, l)
		}
	}
	return out
}

func (f *fakeLedger) PendingLoans(_ context.Context, userID uuid.UUID) ([]model.InboxLoan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.InboxLoan{}
	for id, q := range f.share.requests {
		if q.to != userID || q.status != "pending" {
			continue
		}
		e := f.entryByID(q.from, q.fromEntry)
		var delta int64
		for _, l := range f.contactLinesTo(q.from, e, userID) {
			delta += l.Debit - l.Credit
		}
		out = append(out, model.InboxLoan{ID: id, FromName: f.share.user(q.from).Name, Kind: e.Kind, Amount: q.amount,
			Delta: delta, Date: e.Date, CreatedAt: q.created})
	}
	return out, nil
}

func (f *fakeLedger) LoanSource(_ context.Context, requestID, userID uuid.UUID) (*model.LoanSource, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	q, ok := f.share.requests[requestID]
	if !ok || q.to != userID {
		return nil, repository.ErrNotFound
	}
	e := f.entryByID(q.from, q.fromEntry)
	from := f.share.user(q.from)
	src := &model.LoanSource{Status: q.status, FromUserID: q.from, FromName: from.Name, FromEmail: from.Email,
		Kind: e.Kind, Date: e.EntryDate, Lines: f.contactLinesTo(q.from, e, userID)}
	if q.status == "approved" {
		src.ToEntryID = &q.toEntry
	}
	return src, nil
}

func (f *fakeLedger) ContactForUser(_ context.Context, userID, linked uuid.UUID, name string) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id := range f.contacts[userID] {
		if f.links[id] == linked {
			return id, nil
		}
	}
	if f.contacts[userID] == nil {
		f.contacts[userID] = map[uuid.UUID]string{}
	}
	id := uuid.New()
	f.contacts[userID][id] = name
	f.links[id] = linked
	return id, nil
}

func (f *fakeLedger) ApproveLoan(_ context.Context, requestID, userID uuid.UUID, e *model.Entry) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	q, ok := f.share.requests[requestID]
	if !ok || q.to != userID {
		return uuid.Nil, repository.ErrNotFound
	}
	switch q.status {
	case "approved":
		return q.toEntry, nil
	case "rejected":
		return uuid.Nil, repository.ErrLoanNotPending
	}
	// Inserted directly, like repository.ApproveLoan: a mirror never creates a request back.
	e.ID, e.EntryNo, e.CreatedAt = uuid.New(), int64(len(f.entries[userID])+1), time.Now()
	f.entries[userID] = append(f.entries[userID], e)
	q.status, q.toEntry = "approved", e.ID
	return e.ID, nil
}

func (f *fakeLedger) RejectLoan(_ context.Context, requestID, userID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	q, ok := f.share.requests[requestID]
	if !ok || q.to != userID {
		return repository.ErrNotFound
	}
	if q.status != "pending" {
		return repository.ErrLoanNotPending
	}
	q.status = "rejected"
	return nil
}

// ---- tests ----

func groupOf(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	return body["data"].(map[string]any)
}

func TestGroupAccessAndPrivacy(t *testing.T) {
	r, _ := newTestServer()
	lida := signUp(t, r, "lida@x.com")
	fikri := signUp(t, r, "fikri@x.com")
	budi := signUp(t, r, "budi@x.com")
	for _, tok := range []string{lida, fikri, budi} {
		openBooks(t, r, tok, "2026-09-01")
	}

	code, body := call(t, r, "POST", "/api/v1/groups", lida, `{"name":" Rumah "}`)
	expect(t, "create group", code, 201, body)
	gid := groupOf(t, body)["id"].(string)

	// Registered and unregistered emails get identical answers and both show as pending.
	code, known := call(t, r, "POST", "/api/v1/groups/"+gid+"/invites", lida, `{"email":"Fikri@X.com"}`)
	expect(t, "invite registered email", code, 201, known)
	code, unknown := call(t, r, "POST", "/api/v1/groups/"+gid+"/invites", lida, `{"email":"nobody@x.com"}`)
	expect(t, "invite unknown email", code, 201, unknown)
	if len(groupOf(t, known)) != len(groupOf(t, unknown)) {
		t.Fatalf("invite responses differ in shape: %v vs %v", known, unknown)
	}
	code, body = call(t, r, "GET", "/api/v1/groups", lida, "")
	if pending := body["data"].([]any)[0].(map[string]any)["pending_invites"].([]any); len(pending) != 2 {
		t.Fatalf("both invites should be pending: %v", pending)
	}

	summary := "/api/v1/groups/" + gid + "/summary?date=2026-10-16"
	code, body = call(t, r, "GET", summary, budi, "")
	expect(t, "outsider reads summary", code, 404, body)
	code, body = call(t, r, "GET", summary, fikri, "")
	expect(t, "invited but not accepted reads summary", code, 404, body)
	code, body = call(t, r, "POST", "/api/v1/groups/"+gid+"/invites", budi, `{"email":"budi@x.com"}`)
	expect(t, "outsider invites into the group", code, 404, body)

	code, body = call(t, r, "GET", "/api/v1/inbox", fikri, "")
	invites := groupOf(t, body)["invites"].([]any)
	if len(invites) != 1 {
		t.Fatalf("fikri should see one invite: %v", body)
	}
	inviteID := invites[0].(map[string]any)["id"].(string)
	code, body = call(t, r, "POST", "/api/v1/invites/"+inviteID+"/accept", budi, "")
	expect(t, "accept someone else's invite", code, 404, body)
	code, body = call(t, r, "POST", "/api/v1/invites/"+inviteID+"/accept", fikri, "")
	expect(t, "accept invite", code, 204, body)

	acc := accountIDs(t, r, fikri)
	call(t, r, "POST", "/api/v1/entries", fikri, entryBody("income", "2026-10-01", 1000000000, acc["4001"], acc["1001"]))
	code, body = call(t, r, "GET", summary, lida, "")
	expect(t, "member reads summary", code, 200, body)
	rows := body["data"].([]any)
	if len(rows) != 2 {
		t.Fatalf("summary should list both members: %v", rows)
	}
	for _, row := range rows {
		row := row.(map[string]any)
		if _, leaked := row["entries"]; leaked {
			t.Fatal("summary must not contain entries")
		}
		if row["member"].(map[string]any)["email"] == "fikri@x.com" && row["income"] != float64(1000000000) {
			t.Fatalf("fikri income in summary: %v", row)
		}
	}

	code, body = call(t, r, "POST", "/api/v1/groups/"+gid+"/leave", fikri, "")
	expect(t, "leave", code, 204, body)
	code, body = call(t, r, "GET", summary, fikri, "")
	expect(t, "summary after leaving", code, 404, body)
	code, body = call(t, r, "GET", summary, lida, "")
	if n := len(body["data"].([]any)); n != 1 {
		t.Fatalf("after fikri left, lida should see only herself, got %d", n)
	}
}

func TestSharedLoanMirroring(t *testing.T) {
	r, _ := newTestServer()
	lida := signUp(t, r, "lida@x.com")
	fikri := signUp(t, r, "fikri@x.com")
	budi := signUp(t, r, "budi@x.com")
	for _, tok := range []string{lida, fikri, budi} {
		openBooks(t, r, tok, "2026-09-01")
	}
	lidaAcc, fikriAcc := accountIDs(t, r, lida), accountIDs(t, r, fikri)

	_, body := call(t, r, "POST", "/api/v1/groups", lida, `{"name":"Rumah"}`)
	gid := groupOf(t, body)["id"].(string)
	call(t, r, "POST", "/api/v1/groups/"+gid+"/invites", lida, `{"email":"fikri@x.com"}`)
	_, body = call(t, r, "GET", "/api/v1/inbox", fikri, "")
	call(t, r, "POST", "/api/v1/invites/"+groupOf(t, body)["invites"].([]any)[0].(map[string]any)["id"].(string)+"/accept", fikri, "")

	_, body = call(t, r, "GET", "/api/v1/groups", lida, "")
	var fikriID string
	for _, m := range body["data"].([]any)[0].(map[string]any)["members"].([]any) {
		if m := m.(map[string]any); m["email"] == "fikri@x.com" {
			fikriID = m["id"].(string)
		}
	}
	_, body = call(t, r, "GET", "/api/v1/me", budi, "")
	budiID := groupOf(t, body)["id"].(string)

	_, body = call(t, r, "POST", "/api/v1/contacts", lida, `{"name":"Fikri"}`)
	contact := groupOf(t, body)["id"].(string)
	code, body := call(t, r, "PUT", "/api/v1/contacts/"+contact+"/link", lida, `{"user_id":"`+budiID+`"}`)
	expect(t, "link to someone outside my groups", code, 404, body)
	code, body = call(t, r, "PUT", "/api/v1/contacts/"+contact+"/link", lida, `{"user_id":"`+fikriID+`"}`)
	expect(t, "link to a group member", code, 200, body)

	// Lida pays a 100.000 dinner, her share 40.000: Fikri owes her 60.000.
	code, body = call(t, r, "POST", "/api/v1/loans", lida, loanBody("talangan", contact, lidaAcc["1001"], 10000000,
		`,"my_share":4000000,"expense_account_id":"`+lidaAcc["5102"]+`"`))
	expect(t, "split bill with a linked contact", code, 201, body)
	lidaEntry := groupOf(t, body)["id"].(string)

	_, body = call(t, r, "GET", "/api/v1/inbox", fikri, "")
	loans := groupOf(t, body)["loans"].([]any)
	if len(loans) != 1 {
		t.Fatalf("fikri should have one pending loan: %v", body)
	}
	loan := loans[0].(map[string]any)
	if loan["amount"] != float64(6000000) || loan["delta"] != float64(6000000) || loan["from_name"] != "Lida" {
		t.Fatalf("pending loan: %v", loan)
	}
	reqID := loan["id"].(string)

	code, body = call(t, r, "POST", "/api/v1/loan-requests/"+reqID+"/approve", budi, `{"account_id":"`+fikriAcc["5102"]+`"}`)
	expect(t, "approve someone else's request", code, 404, body)
	code, body = call(t, r, "POST", "/api/v1/loan-requests/"+reqID+"/approve", fikri, `{"account_id":"`+fikriAcc["2101"]+`"}`)
	expect(t, "approve against a per-contact account", code, 400, body)

	// Fikri records his dinner as his own Makan expense; approving twice posts once.
	code, body = call(t, r, "POST", "/api/v1/loan-requests/"+reqID+"/approve", fikri, `{"account_id":"`+fikriAcc["5102"]+`"}`)
	expect(t, "approve", code, 200, body)
	first := groupOf(t, body)["to_entry_id"]
	code, body = call(t, r, "POST", "/api/v1/loan-requests/"+reqID+"/approve", fikri, `{"account_id":"`+fikriAcc["5102"]+`"}`)
	expect(t, "approve again", code, 200, body)
	if groupOf(t, body)["to_entry_id"] != first {
		t.Fatalf("second approve returned another entry: %v vs %v", groupOf(t, body)["to_entry_id"], first)
	}
	_, body = call(t, r, "GET", "/api/v1/entries", fikri, "")
	if body["total"] != float64(1) {
		t.Fatalf("fikri should have exactly one mirror entry, total %v", body["total"])
	}
	mirror := body["data"].([]any)[0].(map[string]any)
	var debit, credit float64
	codes := map[string]float64{}
	for _, l := range mirror["lines"].([]any) {
		l := l.(map[string]any)
		debit += l["debit"].(float64)
		credit += l["credit"].(float64)
		codes[l["code"].(string)] += l["debit"].(float64) - l["credit"].(float64)
	}
	if debit != credit || codes["5102"] != 6000000 || codes["2101"] != -6000000 {
		t.Fatalf("mirror should be Dr Makan 60.000 / Cr Utang 60.000: %v", mirror)
	}

	_, body = call(t, r, "GET", "/api/v1/contacts", fikri, "")
	c := body["data"].([]any)[0].(map[string]any)
	if c["name"] != "Lida" || c["payable"] != float64(6000000) || c["linked_user_id"] == nil {
		t.Fatalf("fikri's auto-created contact: %v", c)
	}

	code, body = call(t, r, "DELETE", "/api/v1/entries/"+lidaEntry, lida, "")
	expect(t, "delete mirrored source entry", code, 409, body)
	code, body = call(t, r, "DELETE", "/api/v1/entries/"+first.(string), fikri, "")
	expect(t, "delete mirror entry", code, 409, body)

	// Fikri pays back; Lida records it; Fikri rejects (he already logged it differently).
	code, body = call(t, r, "POST", "/api/v1/loans", lida, loanBody("lunas_masuk", contact, lidaAcc["1001"], 6000000, ""))
	expect(t, "repayment from a linked contact", code, 201, body)
	repayEntry := groupOf(t, body)["id"].(string)
	_, body = call(t, r, "GET", "/api/v1/inbox", fikri, "")
	repay := groupOf(t, body)["loans"].([]any)[0].(map[string]any)
	if repay["delta"] != float64(-6000000) {
		t.Fatalf("repayment lowers what fikri owes: %v", repay)
	}
	code, body = call(t, r, "POST", "/api/v1/loan-requests/"+repay["id"].(string)+"/reject", fikri, "")
	expect(t, "reject", code, 204, body)
	code, body = call(t, r, "POST", "/api/v1/loan-requests/"+repay["id"].(string)+"/reject", fikri, "")
	expect(t, "reject twice", code, 409, body)
	code, body = call(t, r, "POST", "/api/v1/loan-requests/"+repay["id"].(string)+"/approve", fikri, `{"account_id":"`+fikriAcc["1001"]+`"}`)
	expect(t, "approve after reject", code, 409, body)
	code, body = call(t, r, "DELETE", "/api/v1/entries/"+repayEntry, lida, "")
	expect(t, "delete entry whose request was rejected", code, 204, body)

	// Once they no longer share a group, a linked loan is refused until the contact is unlinked.
	call(t, r, "POST", "/api/v1/groups/"+gid+"/leave", fikri, "")
	code, body = call(t, r, "POST", "/api/v1/loans", lida, loanBody("talangan", contact, lidaAcc["1001"], 100, ""))
	expect(t, "loan to a linked contact outside any shared group", code, 409, body)
	code, body = call(t, r, "PUT", "/api/v1/contacts/"+contact+"/link", lida, `{"user_id":null}`)
	expect(t, "unlink", code, 200, body)
	code, body = call(t, r, "POST", "/api/v1/loans", lida, loanBody("talangan", contact, lidaAcc["1001"], 100, ""))
	expect(t, "loan after unlinking", code, 201, body)
}
