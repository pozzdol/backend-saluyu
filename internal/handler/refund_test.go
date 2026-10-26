package handler

import "testing"

// A refund is income from an expense category: it lowers what the category spent, and the
// budget with it, without counting as income.
func TestRefundLowersSpending(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "lida@x.com")
	acc := accountIDs(t, r, tok)
	bank, makan := acc["1001"], acc["5102"]
	openBooks(t, r, tok, "2026-09-01")

	post(t, r, tok, entryBody("expense", "2026-10-05", 30000000, bank, makan))
	post(t, r, tok, entryBody("income", "2026-10-06", 10000000, makan, bank))

	code, body := call(t, r, "PUT", "/api/v1/budgets/"+makan, tok, `{"date":"2026-10-16","amount":50000000}`)
	expect(t, "set budget", code, 200, body)
	wantNum(t, "budget after refund", body["data"].(map[string]any), "actual", 20000000)

	d := getData(t, r, tok, "/api/v1/dashboard?date=2026-10-16")
	wantNum(t, "dashboard", d, "income", 0)
	wantNum(t, "dashboard", d, "expense", 20000000)

	// Money still cannot go out of an income source.
	code, body = call(t, r, "POST", "/api/v1/entries", tok, entryBody("expense", "2026-10-06", 100, bank, acc["4001"]))
	expect(t, "expense to a revenue account", code, 400, body)
}

// The lending limit rides on the contact's PATCH: nil keeps it, 0 removes it.
func TestContactLimit(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "lida@x.com")

	code, body := call(t, r, "POST", "/api/v1/contacts", tok, `{"name":"Bau"}`)
	expect(t, "create contact", code, 201, body)
	bau := body["data"].(map[string]any)["id"].(string)

	limit := func(step, patch string) any {
		t.Helper()
		code, body := call(t, r, "PATCH", "/api/v1/contacts/"+bau, tok, patch)
		expect(t, step, code, 200, body)
		return body["data"].(map[string]any)["limit"]
	}
	if got := limit("set limit", `{"name":"Bau","limit":50000000}`); got != float64(50000000) {
		t.Fatalf("limit = %v, want 50000000", got)
	}
	if got := limit("rename keeps limit", `{"name":"Bau Bau"}`); got != float64(50000000) {
		t.Fatalf("limit after rename = %v, want 50000000", got)
	}
	if got := limit("clear limit", `{"name":"Bau Bau","limit":0}`); got != nil {
		t.Fatalf("limit after clearing = %v, want null", got)
	}
	code, body = call(t, r, "PATCH", "/api/v1/contacts/"+bau, tok, `{"name":"Bau","limit":-1}`)
	expect(t, "negative limit", code, 400, body)
}
