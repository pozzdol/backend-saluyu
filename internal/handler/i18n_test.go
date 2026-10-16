package handler

import (
	"testing"
	"time"
)

// The frontend translates errors by code, so these codes are a contract with it.
func TestErrorCodes(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "lida@x.com")
	acc := accountIDs(t, r, tok)
	openBooks(t, r, tok, "2026-09-01")

	_, body := call(t, r, "POST", "/api/v1/register", "", `{"name":"Lida","email":"lida@x.com","password":"secret123"}`)
	if body["code"] != "email_taken" {
		t.Fatalf("duplicate register code: %v", body)
	}

	code, body := call(t, r, "POST", "/api/v1/register", "", `{"name":"Li","email":"not-an-email","password":""}`)
	expect(t, "invalid register", code, 400, body)
	codes := body["field_codes"].(map[string]any)
	if body["code"] != "validation" || codes["name"] != "min_len:3" || codes["email"] != "email" || codes["password"] != "required" {
		t.Fatalf("field codes: %v", body)
	}
	if body["fields"].(map[string]any)["email"] != "must be a valid email" {
		t.Fatalf("English field text kept as fallback: %v", body)
	}

	_, body = call(t, r, "POST", "/api/v1/entries", tok, entryBody("expense", "2026-10-01", 100, acc["1001"], acc["4001"]))
	if body["code"] != "wrong_account_type" {
		t.Fatalf("wrong account type code: %v", body)
	}
	_, body = call(t, r, "POST", "/api/v1/entries", tok, `{bad`)
	if body["code"] != "invalid_body" {
		t.Fatalf("invalid body code: %v", body)
	}
}

func TestLocale(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "lida@x.com")

	_, body := call(t, r, "GET", "/api/v1/me", tok, "")
	if body["data"].(map[string]any)["locale"] != nil {
		t.Fatalf("new user should follow the browser (null locale): %v", body)
	}
	code, body := call(t, r, "PATCH", "/api/v1/me", tok, `{"locale":"id"}`)
	expect(t, "set locale", code, 200, body)
	if body["data"].(map[string]any)["locale"] != "id" {
		t.Fatalf("locale not saved: %v", body)
	}
	code, body = call(t, r, "PATCH", "/api/v1/me", tok, `{"locale":"fr"}`)
	expect(t, "unsupported locale", code, 400, body)
	code, body = call(t, r, "PATCH", "/api/v1/me", tok, `{"locale":"auto"}`)
	expect(t, "back to browser", code, 200, body)
	if body["data"].(map[string]any)["locale"] != nil {
		t.Fatalf("auto should clear locale: %v", body)
	}
}

func TestAccountIconsAndUsage(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "lida@x.com")
	acc := accountIDs(t, r, tok)
	openBooks(t, r, tok, "2020-01-01")

	today := time.Now().Format(time.DateOnly)
	old := time.Now().AddDate(0, 0, -45).Format(time.DateOnly)
	call(t, r, "POST", "/api/v1/entries", tok, entryBody("expense", today, 100, acc["1001"], acc["5102"]))
	call(t, r, "POST", "/api/v1/entries", tok, entryBody("expense", today, 100, acc["1001"], acc["5102"]))
	call(t, r, "POST", "/api/v1/entries", tok, entryBody("expense", old, 100, acc["1001"], acc["5101"]))

	_, body := call(t, r, "GET", "/api/v1/accounts", tok, "")
	by := map[string]map[string]any{}
	for _, a := range body["data"].([]any) {
		a := a.(map[string]any)
		by[a["code"].(string)] = a
	}
	if by["5102"]["icon"] != "utensils" || by["5102"]["color"] != "orange" {
		t.Fatalf("template icon for Makan: %v", by["5102"])
	}
	if by["5102"]["uses_30d"] != float64(2) || by["5101"]["uses_30d"] != float64(0) {
		t.Fatalf("uses_30d counts only the last 30 days: makan %v transport %v", by["5102"]["uses_30d"], by["5101"]["uses_30d"])
	}

	code, body := call(t, r, "POST", "/api/v1/accounts", tok, `{"code":"5108","name":"Kopi","type":"Expense","icon":"coffee","color":"amber"}`)
	expect(t, "create with icon", code, 201, body)
	code, body = call(t, r, "POST", "/api/v1/accounts", tok, `{"code":"5109","name":"X","type":"Expense","icon":"rocket"}`)
	expect(t, "unknown icon", code, 400, body)
	code, body = call(t, r, "PATCH", "/api/v1/accounts/"+acc["5103"], tok, `{"icon":"coffee","color":"pink"}`)
	expect(t, "change icon", code, 200, body)
	if body["data"].(map[string]any)["icon"] != "coffee" {
		t.Fatalf("icon not updated: %v", body)
	}
}
