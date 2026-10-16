package handler

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/repository"
)

// ---- fakeLedger: edits, revisions and change requests ----

type fakeChange struct {
	loanReq   uuid.UUID
	by        uuid.UUID
	proposal  model.ChangeProposal
	status    string
	createdAt time.Time
}

func cloneEntry(e *model.Entry) *model.Entry {
	c := *e
	c.Lines = append([]model.Line(nil), e.Lines...)
	return &c
}

// revise records a revision and swaps after into before's place. Caller holds f.mu.
func (f *fakeLedger) revise(author uuid.UUID, before, after *model.Entry) {
	after.ID, after.EntryNo, after.CreatedAt = before.ID, before.EntryNo, before.CreatedAt
	after.Date = after.EntryDate.Format(time.DateOnly)
	name := ""
	if f.share.users != nil {
		name = f.share.user(author).Name
	}
	f.revs[before.ID] = append(f.revs[before.ID], model.Revision{ChangedAt: time.Now(), ChangedBy: name,
		Before: model.Snapshot(before), After: model.Snapshot(after)})
	for uid, list := range f.entries {
		for i, e := range list {
			if e.ID == before.ID {
				f.entries[uid][i] = after
			}
		}
	}
}

func (f *fakeLedger) GetEntry(_ context.Context, userID, id uuid.UUID) (*model.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.entryByID(userID, id); e != nil {
		return cloneEntry(e), nil
	}
	return nil, repository.ErrEntryNotFound
}

func (f *fakeLedger) LoanLink(_ context.Context, entryID uuid.UUID) (string, uuid.UUID, bool, uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, q := range f.share.requests {
		if q.fromEntry == entryID {
			return q.status, id, true, q.to, nil
		}
		if q.toEntry == entryID {
			return q.status, id, false, q.from, nil
		}
	}
	return "", uuid.Nil, false, uuid.Nil, nil
}

func (f *fakeLedger) ReplaceEntry(_ context.Context, userID uuid.UUID, before, after *model.Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revise(userID, before, after)
	for id, q := range f.share.requests {
		if q.fromEntry == after.ID && q.status != "approved" {
			delete(f.share.requests, id)
		}
	}
	f.createLoanRequest(userID, after)
	return nil
}

func (f *fakeLedger) Revisions(_ context.Context, entryID uuid.UUID) ([]model.Revision, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]model.Revision{}, f.revs[entryID]...), nil
}

func (f *fakeLedger) ProposeChange(_ context.Context, loanRequestID, userID uuid.UUID, p model.ChangeProposal) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.changes {
		if c.loanReq == loanRequestID && c.status == "pending" {
			c.status = "cancelled"
		}
	}
	f.changes[uuid.New()] = &fakeChange{loanReq: loanRequestID, by: userID, proposal: p, status: "pending", createdAt: time.Now()}
	return nil
}

func (f *fakeLedger) ChangePending(_ context.Context, loanRequestID uuid.UUID) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.changes {
		if c.loanReq == loanRequestID && c.status == "pending" {
			return true, nil
		}
	}
	return false, nil
}

func absInt(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func (f *fakeLedger) PendingChanges(_ context.Context, userID uuid.UUID) ([]model.InboxChange, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.InboxChange{}
	for id, c := range f.changes {
		q := f.share.requests[c.loanReq]
		if c.status != "pending" || q.to != userID {
			continue
		}
		var net int64
		for _, l := range c.proposal.MirrorLines {
			net += l.Debit - l.Credit
		}
		out = append(out, model.InboxChange{ID: id, FromName: f.share.user(q.from).Name, CreatedAt: c.createdAt,
			Before: model.AmountDate{Amount: q.amount, Date: f.entryByID(q.from, q.fromEntry).Date},
			After:  model.AmountDate{Amount: absInt(net), Date: c.proposal.Entry.Date}})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (f *fakeLedger) ChangeSource(_ context.Context, changeID, userID uuid.UUID) (*model.ChangeSource, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.changes[changeID]
	if !ok || f.share.requests[c.loanReq].to != userID {
		return nil, repository.ErrNotFound
	}
	q := f.share.requests[c.loanReq]
	return &model.ChangeSource{Status: c.status, LoanRequestID: c.loanReq, FromUserID: q.from, FromName: f.share.user(q.from).Name,
		FromEntryID: q.fromEntry, ToEntryID: q.toEntry, Proposal: c.proposal}, nil
}

func (f *fakeLedger) ApplyChange(_ context.Context, changeID, approver, sender uuid.UUID, senderBefore, senderAfter, mirrorBefore, mirrorAfter *model.Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.changes[changeID]
	switch c.status {
	case "approved":
		return nil
	case "pending":
	default:
		return repository.ErrLoanNotPending
	}
	f.revise(sender, senderBefore, senderAfter)
	f.revise(approver, mirrorBefore, mirrorAfter)
	var net int64
	for _, l := range mirrorAfter.Lines {
		if l.ContactID != nil {
			net += l.Debit - l.Credit
		}
	}
	f.share.requests[c.loanReq].amount = absInt(net)
	c.status = "approved"
	return nil
}

func (f *fakeLedger) RejectChange(_ context.Context, changeID, userID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.changes[changeID]
	if !ok || f.share.requests[c.loanReq].to != userID {
		return repository.ErrNotFound
	}
	if c.status != "pending" {
		return repository.ErrLoanNotPending
	}
	c.status = "rejected"
	return nil
}

// ---- tests ----

func TestEditSimpleEntry(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "lida@x.com")
	acc := accountIDs(t, r, tok)
	openBooks(t, r, tok, "2026-09-01")

	_, body := call(t, r, "POST", "/api/v1/entries", tok, entryBody("expense", "2026-10-05", 2500000, acc["1001"], acc["5102"]))
	id := body["data"].(map[string]any)["id"].(string)
	no := body["data"].(map[string]any)["entry_no"]

	// Typo fix: 25.000 was 35.000, and it was Jajan, not Makan.
	code, body := call(t, r, "PUT", "/api/v1/entries/"+id, tok, entryBody("expense", "2026-10-06", 3500000, acc["1001"], acc["5103"]))
	expect(t, "edit expense", code, 200, body)
	e := body["data"].(map[string]any)
	if e["id"] != id || e["entry_no"] != no || e["entry_date"] != "2026-10-06" {
		t.Fatalf("edit must keep id and number and change the date: %v", e)
	}

	_, body = call(t, r, "GET", "/api/v1/accounts/"+acc["5102"]+"/ledger", tok, "")
	if body["data"].(map[string]any)["closing"] != float64(0) {
		t.Fatalf("Makan should be back to 0: %v", body["data"])
	}
	_, body = call(t, r, "GET", "/api/v1/accounts/"+acc["5103"]+"/ledger", tok, "")
	if body["data"].(map[string]any)["closing"] != float64(3500000) {
		t.Fatalf("Jajan should hold 35.000: %v", body["data"])
	}
	_, body = call(t, r, "GET", "/api/v1/reports/trial-balance?as_of=2026-10-31", tok, "")
	if body["data"].(map[string]any)["balanced"] != true {
		t.Fatal("trial balance after edit")
	}

	code, body = call(t, r, "GET", "/api/v1/entries/"+id, tok, "")
	expect(t, "entry detail", code, 200, body)
	revs := body["data"].(map[string]any)["revisions"].([]any)
	if len(revs) != 1 {
		t.Fatalf("want 1 revision, got %v", revs)
	}
	before := revs[0].(map[string]any)["before"].(map[string]any)
	if before["entry_date"] != "2026-10-05" || before["lines"].([]any)[0].(map[string]any)["debit"] != float64(2500000) {
		t.Fatalf("revision before: %v", before)
	}

	code, body = call(t, r, "PUT", "/api/v1/entries/"+id, tok, entryBody("income", "2026-10-06", 100, acc["4001"], acc["1001"]))
	expect(t, "change kind", code, 400, body)
	if body["code"] != "kind_change" {
		t.Fatalf("kind change code: %v", body)
	}
	code, body = call(t, r, "PUT", "/api/v1/entries/"+id, tok, entryBody("expense", "2026-08-01", 100, acc["1001"], acc["5103"]))
	expect(t, "edit to before start date", code, 400, body)
}

func TestEditLoanAndLines(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "lida@x.com")
	acc := accountIDs(t, r, tok)
	openBooks(t, r, tok, "2026-09-01")
	_, body := call(t, r, "POST", "/api/v1/contacts", tok, `{"name":"Bau"}`)
	bau := body["data"].(map[string]any)["id"].(string)

	call(t, r, "POST", "/api/v1/loans", tok, loanBody("talangan", bau, acc["1001"], 6000000, ""))
	_, body = call(t, r, "POST", "/api/v1/loans", tok, loanBody("lunas_masuk", bau, acc["1001"], 6000000, ""))
	repay := body["data"].(map[string]any)["id"].(string)

	// Lowering the repayment must not count its own old 60.000 against itself.
	code, body := call(t, r, "PUT", "/api/v1/loans/"+repay, tok, loanBody("lunas_masuk", bau, acc["1001"], 5000000, ""))
	expect(t, "edit repayment down", code, 200, body)
	_, body = call(t, r, "GET", "/api/v1/contacts/"+bau, tok, "")
	if c := body["data"].(map[string]any)["contact"].(map[string]any); c["receivable"] != float64(1000000) {
		t.Fatalf("after edit Bau still owes 10.000: %v", c)
	}
	code, body = call(t, r, "PUT", "/api/v1/loans/"+repay, tok, loanBody("talangan", bau, acc["1001"], 100, ""))
	expect(t, "repayment turned into a loan", code, 400, body)
	code, body = call(t, r, "PUT", "/api/v1/entries/"+repay, tok, entryBody("expense", "2026-10-05", 100, acc["1001"], acc["5102"]))
	expect(t, "loan through the simple form", code, 400, body)
	if body["code"] != "wrong_edit_form" {
		t.Fatalf("wrong form code: %v", body)
	}

	// Cash count edited line by line.
	call(t, r, "PUT", "/api/v1/opening", tok, fmt.Sprintf(`{"start_date":"2026-09-01","items":[{"account_id":%q,"amount":60000000}]}`, acc["1002"]))
	_, body = call(t, r, "POST", "/api/v1/cash-counts", tok, `{"account_id":"`+acc["1002"]+`","date":"2026-10-10","actual":54000000}`)
	cc := body["data"].(map[string]any)["entry"].(map[string]any)["id"].(string)
	code, body = call(t, r, "PUT", "/api/v1/entries/"+cc+"/lines", tok, fmt.Sprintf(
		`{"entry_date":"2026-10-10","description":"Hitung kas","lines":[{"account_id":%q,"debit":5000000},{"account_id":%q,"credit":5000000}]}`, acc["5199"], acc["1002"]))
	expect(t, "edit cash count lines", code, 200, body)
	code, body = call(t, r, "PUT", "/api/v1/entries/"+cc+"/lines", tok, fmt.Sprintf(
		`{"entry_date":"2026-10-10","lines":[{"account_id":%q,"debit":5000000},{"account_id":%q,"credit":4000000}]}`, acc["5199"], acc["1002"]))
	expect(t, "unbalanced lines", code, 400, body)
	if body["code"] != "unbalanced" {
		t.Fatalf("unbalanced code: %v", body)
	}
}

func TestOpeningEditableWithEntries(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "lida@x.com")
	acc := accountIDs(t, r, tok)
	call(t, r, "PUT", "/api/v1/opening", tok, fmt.Sprintf(`{"start_date":"2026-10-01","items":[{"account_id":%q,"amount":100000000}]}`, acc["1001"]))
	call(t, r, "POST", "/api/v1/entries", tok, entryBody("expense", "2026-10-05", 100, acc["1001"], acc["5102"]))

	code, body := call(t, r, "PUT", "/api/v1/opening", tok, fmt.Sprintf(`{"start_date":"2026-10-01","items":[{"account_id":%q,"amount":120000000}]}`, acc["1001"]))
	expect(t, "edit opening after transactions", code, 200, body)
	if body["data"].(map[string]any)["editable"] != true {
		t.Fatalf("opening stays editable: %v", body)
	}
	code, body = call(t, r, "PUT", "/api/v1/opening", tok, `{"start_date":"2026-10-06","items":[]}`)
	expect(t, "start date after the first transaction", code, 409, body)
	if body["code"] != "start_after_entries" {
		t.Fatalf("code: %v", body)
	}
	_, body = call(t, r, "GET", "/api/v1/reports/trial-balance?as_of=2026-10-31", tok, "")
	if tb := body["data"].(map[string]any); tb["balanced"] != true || tb["total_debit"] != float64(120000000) {
		t.Fatalf("opening edit reflected in trial balance: %v", tb)
	}
}

func TestEditApprovedSharedLoan(t *testing.T) {
	r, _ := newTestServer()
	lida := signUp(t, r, "lida@x.com")
	fikri := signUp(t, r, "fikri@x.com")
	openBooks(t, r, lida, "2026-09-01")
	openBooks(t, r, fikri, "2026-09-01")
	la, fa := accountIDs(t, r, lida), accountIDs(t, r, fikri)

	_, body := call(t, r, "POST", "/api/v1/groups", lida, `{"name":"Rumah"}`)
	gid := body["data"].(map[string]any)["id"].(string)
	call(t, r, "POST", "/api/v1/groups/"+gid+"/invites", lida, `{"email":"fikri@x.com"}`)
	_, body = call(t, r, "GET", "/api/v1/inbox", fikri, "")
	call(t, r, "POST", "/api/v1/invites/"+body["data"].(map[string]any)["invites"].([]any)[0].(map[string]any)["id"].(string)+"/accept", fikri, "")
	_, body = call(t, r, "GET", "/api/v1/me", fikri, "")
	fikriID := body["data"].(map[string]any)["id"].(string)
	_, body = call(t, r, "POST", "/api/v1/contacts", lida, `{"name":"Fikri"}`)
	contact := body["data"].(map[string]any)["id"].(string)
	call(t, r, "PUT", "/api/v1/contacts/"+contact+"/link", lida, `{"user_id":"`+fikriID+`"}`)

	_, body = call(t, r, "POST", "/api/v1/loans", lida, loanBody("talangan", contact, la["1001"], 6000000, ""))
	entry := body["data"].(map[string]any)["id"].(string)
	_, body = call(t, r, "GET", "/api/v1/inbox", fikri, "")
	req := body["data"].(map[string]any)["loans"].([]any)[0].(map[string]any)["id"].(string)
	_, body = call(t, r, "POST", "/api/v1/loan-requests/"+req+"/approve", fikri, `{"account_id":"`+fa["5102"]+`"}`)
	mirror := body["data"].(map[string]any)["to_entry_id"].(string)

	payable := func() float64 {
		_, body := call(t, r, "GET", "/api/v1/contacts", fikri, "")
		return body["data"].([]any)[0].(map[string]any)["payable"].(float64)
	}

	// Lida corrects 60.000 to 75.000: it waits for Fikri, nothing changes yet.
	code, body := call(t, r, "PUT", "/api/v1/loans/"+entry, lida, loanBody("talangan", contact, la["1001"], 7500000, ""))
	expect(t, "edit approved loan", code, 202, body)
	if body["status"] != "pending_approval" {
		t.Fatalf("status: %v", body)
	}
	_, body = call(t, r, "GET", "/api/v1/entries/"+entry, lida, "")
	if d := body["data"].(map[string]any); d["change_pending"] != true || len(d["revisions"].([]any)) != 0 {
		t.Fatalf("sender sees a pending change and no revision yet: %v", d)
	}
	if payable() != 6000000 {
		t.Fatal("mirror must not change before approval")
	}

	// Fikri cannot change the amount on his side.
	code, body = call(t, r, "PUT", "/api/v1/entries/"+mirror+"/lines", fikri, "{}")
	expect(t, "lines editor on a loan", code, 400, body)

	_, body = call(t, r, "GET", "/api/v1/inbox", fikri, "")
	changes := body["data"].(map[string]any)["changes"].([]any)
	if len(changes) != 1 {
		t.Fatalf("fikri should see one change: %v", body)
	}
	ch := changes[0].(map[string]any)
	if ch["before"].(map[string]any)["amount"] != float64(6000000) || ch["after"].(map[string]any)["amount"] != float64(7500000) {
		t.Fatalf("change amounts: %v", ch)
	}
	changeID := ch["id"].(string)
	code, body = call(t, r, "POST", "/api/v1/loan-change-requests/"+changeID+"/approve", lida, "")
	expect(t, "sender approves own change", code, 404, body)
	code, body = call(t, r, "POST", "/api/v1/loan-change-requests/"+changeID+"/approve", fikri, "")
	expect(t, "approve change", code, 204, body)
	code, body = call(t, r, "POST", "/api/v1/loan-change-requests/"+changeID+"/approve", fikri, "")
	expect(t, "approve change again", code, 204, body)

	if payable() != 7500000 {
		t.Fatalf("mirror after approval: payable %v", payable())
	}
	_, body = call(t, r, "GET", "/api/v1/entries/"+mirror, fikri, "")
	m := body["data"].(map[string]any)
	if len(m["revisions"].([]any)) != 1 {
		t.Fatalf("mirror keeps one revision: %v", m)
	}
	var debit, credit float64
	for _, l := range m["entry"].(map[string]any)["lines"].([]any) {
		l := l.(map[string]any)
		debit += l["debit"].(float64)
		credit += l["credit"].(float64)
		if l["contact_id"] == nil && l["code"] != "5102" {
			t.Fatalf("mirror keeps Fikri's own account (Makan): %v", l)
		}
	}
	if debit != credit || debit != 7500000 {
		t.Fatalf("mirror balanced at 75.000: %v / %v", debit, credit)
	}
	_, body = call(t, r, "GET", "/api/v1/entries/"+entry, lida, "")
	if len(body["data"].(map[string]any)["revisions"].([]any)) != 1 {
		t.Fatal("sender entry keeps one revision after approval")
	}

	// A rejected change leaves both books alone.
	call(t, r, "PUT", "/api/v1/loans/"+entry, lida, loanBody("talangan", contact, la["1001"], 100, ""))
	_, body = call(t, r, "GET", "/api/v1/inbox", fikri, "")
	second := body["data"].(map[string]any)["changes"].([]any)[0].(map[string]any)["id"].(string)
	code, body = call(t, r, "POST", "/api/v1/loan-change-requests/"+second+"/reject", fikri, "")
	expect(t, "reject change", code, 204, body)
	if payable() != 7500000 {
		t.Fatal("reject must not change the mirror")
	}

}
