package handler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/repository"
)

// Contact methods of fakeLedger. Balances are computed from posted lines, like the SQL does,
// and deleting a contact that any line points at fails like the ON DELETE RESTRICT foreign key.

func (f *fakeLedger) ListContacts(_ context.Context, userID uuid.UUID, only *uuid.UUID) ([]model.Contact, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.Contact{}
	for id, name := range f.contacts[userID] {
		if only != nil && *only != id {
			continue
		}
		c := model.Contact{ID: id, Name: name}
		if linked, ok := f.links[id]; ok {
			c.LinkedUserID = &linked
		}
		for _, e := range f.entries[userID] {
			for _, l := range e.Lines {
				if l.ContactID == nil || *l.ContactID != id {
					continue
				}
				switch f.userAccounts(userID)[l.AccountID].Code {
				case "1101":
					c.Receivable += l.Debit - l.Credit
				case "2101":
					c.Payable += l.Credit - l.Debit
				}
			}
		}
		c.Net = c.Receivable - c.Payable
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

func (f *fakeLedger) ContactIDs(_ context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[uuid.UUID]bool{}
	for _, id := range ids {
		if _, ok := f.contacts[userID][id]; ok {
			out[id] = true
		}
	}
	return out, nil
}

func (f *fakeLedger) nameTaken(userID, except uuid.UUID, name string) bool {
	for id, other := range f.contacts[userID] {
		if id != except && strings.EqualFold(other, name) {
			return true
		}
	}
	return false
}

func (f *fakeLedger) CreateContact(_ context.Context, userID uuid.UUID, name string) (*model.Contact, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.nameTaken(userID, uuid.Nil, name) {
		return nil, repository.ErrContactNameTaken
	}
	if f.contacts[userID] == nil {
		f.contacts[userID] = map[uuid.UUID]string{}
	}
	c := model.Contact{ID: uuid.New(), Name: name}
	f.contacts[userID][c.ID] = name
	return &c, nil
}

func (f *fakeLedger) RenameContact(_ context.Context, userID, id uuid.UUID, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.contacts[userID][id]; !ok {
		return repository.ErrContactNotFound
	}
	if f.nameTaken(userID, id, name) {
		return repository.ErrContactNameTaken
	}
	f.contacts[userID][id] = name
	return nil
}

func (f *fakeLedger) DeleteContact(_ context.Context, userID, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.contacts[userID][id]; !ok {
		return repository.ErrContactNotFound
	}
	for _, e := range f.entries[userID] {
		for _, l := range e.Lines {
			if l.ContactID != nil && *l.ContactID == id {
				return repository.ErrContactHasHistory
			}
		}
	}
	delete(f.contacts[userID], id)
	return nil
}

func (f *fakeLedger) ContactHistory(_ context.Context, userID, contactID uuid.UUID) ([]model.ContactLine, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.ContactLine{}
	for _, e := range f.entries[userID] {
		for _, l := range e.Lines {
			if l.ContactID != nil && *l.ContactID == contactID {
				out = append(out, model.ContactLine{EntryID: e.ID, EntryNo: e.EntryNo, Date: e.Date, Kind: e.Kind,
					Description: e.Description, Code: f.userAccounts(userID)[l.AccountID].Code, Debit: l.Debit, Credit: l.Credit})
			}
		}
	}
	return out, nil
}

func loanBody(typ, contact, account string, amount int64, extra string) string {
	return fmt.Sprintf(`{"type":%q,"contact_id":%q,"account_id":%q,"amount":%d,"entry_date":"2026-10-05"%s}`,
		typ, contact, account, amount, extra)
}

func TestContactsAndLoans(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "lida@x.com")
	acc := accountIDs(t, r, tok)
	bank, makan := acc["1001"], acc["5102"]
	openBooks(t, r, tok, "2026-09-01")

	code, body := call(t, r, "POST", "/api/v1/contacts", tok, `{"name":" Bau "}`)
	expect(t, "create contact", code, 201, body)
	bau := body["data"].(map[string]any)["id"].(string)
	code, body = call(t, r, "POST", "/api/v1/contacts", tok, `{"name":"bau"}`)
	expect(t, "same name, other case", code, 409, body)

	position := func(step string) (receivable, payable float64) {
		t.Helper()
		code, body := call(t, r, "GET", "/api/v1/contacts/"+bau, tok, "")
		expect(t, step, code, 200, body)
		c := body["data"].(map[string]any)["contact"].(map[string]any)
		return c["receivable"].(float64), c["payable"].(float64)
	}

	// Split bill: I pay 100.000 for dinner, 40.000 is my meal, Bau owes me 60.000.
	code, body = call(t, r, "POST", "/api/v1/loans", tok, loanBody("talangan", bau, bank, 10000000,
		`,"my_share":4000000,"expense_account_id":"`+makan+`"`))
	expect(t, "split bill", code, 201, body)
	lines := body["data"].(map[string]any)["lines"].([]any)
	if len(lines) != 3 {
		t.Fatalf("split bill should have 3 lines: %v", lines)
	}
	for _, want := range []struct {
		code          string
		debit, credit float64
		contact       bool
	}{{"1101", 6000000, 0, true}, {"5102", 4000000, 0, false}, {"1001", 0, 10000000, false}} {
		found := false
		for _, l := range lines {
			l := l.(map[string]any)
			if l["code"] == want.code && l["debit"] == want.debit && l["credit"] == want.credit && (l["contact_id"] != nil) == want.contact {
				found = true
			}
		}
		if !found {
			t.Fatalf("split bill missing line %+v in %v", want, lines)
		}
	}
	if rec, _ := position("after split bill"); rec != 6000000 {
		t.Fatalf("receivable %v, want 6000000", rec)
	}

	code, body = call(t, r, "POST", "/api/v1/loans", tok, loanBody("talangan", bau, bank, 100, `,"my_share":100`))
	expect(t, "my share equal to the whole amount", code, 400, body)
	code, body = call(t, r, "POST", "/api/v1/loans", tok, loanBody("talangan", bau, bank, 100, `,"my_share":50`))
	expect(t, "my share without an expense account", code, 400, body)

	// Bau pays back 70.000: 10.000 more than owed. Rejected unless confirmed.
	code, body = call(t, r, "POST", "/api/v1/loans", tok, loanBody("lunas_masuk", bau, bank, 7000000, ""))
	expect(t, "overpayment without confirm", code, 409, body)
	code, body = call(t, r, "POST", "/api/v1/loans", tok, loanBody("lunas_masuk", bau, bank, 7000000, `,"confirm_overpay":true`))
	expect(t, "overpayment confirmed", code, 201, body)
	if rec, pay := position("after overpayment"); rec != 0 || pay != 1000000 {
		t.Fatalf("after overpayment receivable %v payable %v, want 0 and 1000000", rec, pay)
	}

	code, body = call(t, r, "DELETE", "/api/v1/contacts/"+bau, tok, "")
	expect(t, "delete contact with balance", code, 409, body)
	code, body = call(t, r, "POST", "/api/v1/loans", tok, loanBody("lunas_keluar", bau, bank, 1000000, ""))
	expect(t, "pay back the extra", code, 201, body)
	if rec, pay := position("settled"); rec != 0 || pay != 0 {
		t.Fatalf("settled receivable %v payable %v", rec, pay)
	}
	code, body = call(t, r, "DELETE", "/api/v1/contacts/"+bau, tok, "")
	expect(t, "delete settled contact with history", code, 409, body)

	code, body = call(t, r, "GET", "/api/v1/contacts/"+bau, tok, "")
	history := body["data"].(map[string]any)["history"].([]any)
	if last := history[len(history)-1].(map[string]any); last["running_net"] != float64(0) {
		t.Fatalf("running net should end at 0: %v", history)
	}

	// Borrowing: Lia lends me 200.000.
	code, body = call(t, r, "POST", "/api/v1/contacts", tok, `{"name":"Lia"}`)
	lia := body["data"].(map[string]any)["id"].(string)
	code, body = call(t, r, "POST", "/api/v1/loans", tok, loanBody("pinjam", lia, bank, 20000000, ""))
	expect(t, "borrow", code, 201, body)
	code, body = call(t, r, "GET", "/api/v1/contacts", tok, "")
	for _, c := range body["data"].([]any) {
		if c := c.(map[string]any); c["name"] == "Lia" && c["net"] != float64(-20000000) {
			t.Fatalf("Lia net %v, want -20000000", c["net"])
		}
	}

	// Money must move through cash/bank, and 1101 can never be posted without a person.
	code, body = call(t, r, "POST", "/api/v1/loans", tok, loanBody("pinjam", lia, makan, 100, ""))
	expect(t, "loan through an expense account", code, 400, body)
	code, body = call(t, r, "POST", "/api/v1/entries", tok, entryBody("income", "2026-10-05", 100, acc["4001"], acc["1101"]))
	expect(t, "receivable without a contact via the simple form", code, 400, body)

	code, body = call(t, r, "POST", "/api/v1/contacts", tok, `{"name":"Kosong"}`)
	empty := body["data"].(map[string]any)["id"].(string)
	code, body = call(t, r, "PATCH", "/api/v1/contacts/"+empty, tok, `{"name":"lia"}`)
	expect(t, "rename to a taken name", code, 409, body)
	code, body = call(t, r, "DELETE", "/api/v1/contacts/"+empty, tok, "")
	expect(t, "delete unused contact", code, 204, body)

	fikri := signUp(t, r, "fikri@x.com")
	openBooks(t, r, fikri, "2026-09-01")
	code, body = call(t, r, "POST", "/api/v1/loans", fikri, loanBody("pinjam", lia, accountIDs(t, r, fikri)["1001"], 100, ""))
	expect(t, "loan with another user's contact", code, 404, body)
	code, body = call(t, r, "GET", "/api/v1/contacts/"+lia, fikri, "")
	expect(t, "read another user's contact", code, 404, body)
}

func TestOpeningWithContacts(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "lida@x.com")
	acc := accountIDs(t, r, tok)
	code, body := call(t, r, "POST", "/api/v1/contacts", tok, `{"name":"Lia"}`)
	lia := body["data"].(map[string]any)["id"].(string)

	code, body = call(t, r, "PUT", "/api/v1/opening", tok,
		`{"start_date":"2026-10-01","items":[{"account_id":"`+acc["1101"]+`","amount":4500000}]}`)
	expect(t, "receivable opening without a contact", code, 400, body)
	code, body = call(t, r, "PUT", "/api/v1/opening", tok,
		`{"start_date":"2026-10-01","items":[{"account_id":"`+acc["1001"]+`","contact_id":"`+lia+`","amount":100}]}`)
	expect(t, "contact on a bank account", code, 400, body)

	// Lia owed me 45.000 before I started; I owed her 10.000 too.
	code, body = call(t, r, "PUT", "/api/v1/opening", tok, fmt.Sprintf(`{"start_date":"2026-10-01","items":[
		{"account_id":%q,"amount":100000000},
		{"account_id":%q,"contact_id":%q,"amount":4500000},
		{"account_id":%q,"contact_id":%q,"amount":1000000}]}`, acc["1001"], acc["1101"], lia, acc["2101"], lia))
	expect(t, "opening with per-contact balances", code, 200, body)
	code, body = call(t, r, "GET", "/api/v1/contacts/"+lia, tok, "")
	c := body["data"].(map[string]any)["contact"].(map[string]any)
	if c["receivable"] != float64(4500000) || c["payable"] != float64(1000000) || c["net"] != float64(3500000) {
		t.Fatalf("opening position: %v", c)
	}
	code, body = call(t, r, "GET", "/api/v1/reports/trial-balance?as_of=2026-10-01", tok, "")
	if body["data"].(map[string]any)["balanced"] != true {
		t.Fatalf("trial balance after opening with contacts: %v", body["data"])
	}
}
