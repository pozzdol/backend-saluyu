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

// fakeCredit holds what the cards and installment tables hold in SQL.
type fakeCredit struct {
	cards    map[uuid.UUID]model.Card        // account id -> card
	plans    map[uuid.UUID]*model.Plan       // plan id -> plan
	owner    map[uuid.UUID]uuid.UUID         // plan id -> user
	payments map[uuid.UUID]map[int]uuid.UUID // plan id -> seq -> entry id
}

func newFakeCredit() *fakeCredit {
	return &fakeCredit{cards: map[uuid.UUID]model.Card{}, plans: map[uuid.UUID]*model.Plan{},
		owner: map[uuid.UUID]uuid.UUID{}, payments: map[uuid.UUID]map[int]uuid.UUID{}}
}

func (f *fakeLedger) NextCode(_ context.Context, userID uuid.UUID, from, to int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	used := map[string]bool{}
	for _, a := range f.userAccounts(userID) {
		used[a.Code] = true
	}
	for n := from; n <= to; n++ {
		if c := fmt.Sprint(n); !used[c] {
			return c, nil
		}
	}
	return "", repository.ErrAccountCodeTaken
}

func (f *fakeLedger) addAccount(userID uuid.UUID, a model.Account) {
	f.userAccounts(userID)[a.ID] = &a
}

func (f *fakeLedger) CreateCard(_ context.Context, userID uuid.UUID, c *model.Card) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addAccount(userID, c.Account)
	f.credit.cards[c.Account.ID] = *c
	return nil
}

func (f *fakeLedger) UpdateCard(_ context.Context, userID, accountID uuid.UUID, s model.CardSettings) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.credit.cards[accountID]
	if _, mine := f.userAccounts(userID)[accountID]; !ok || !mine {
		return repository.ErrNotACard
	}
	c.StatementDay, c.DueDay, c.CreditLimit, c.MinPaymentPct = s.StatementDay, s.DueDay, s.CreditLimit, s.MinPaymentPct
	f.credit.cards[accountID] = c
	return nil
}

func (f *fakeLedger) Cards(_ context.Context, userID uuid.UUID) ([]model.Card, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.Card{}
	for id, c := range f.credit.cards {
		if a, ok := f.userAccounts(userID)[id]; ok {
			c.Account = *a
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Account.Code < out[j].Account.Code })
	return out, nil
}

func (f *fakeLedger) CreatePlan(_ context.Context, userID uuid.UUID, p *model.Plan, borrowing *model.Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addAccount(userID, p.Account)
	p.ID = uuid.New()
	copied := *p
	f.credit.plans[p.ID], f.credit.owner[p.ID] = &copied, userID
	f.credit.payments[p.ID] = map[int]uuid.UUID{}
	if borrowing != nil {
		f.insertEntry(userID, borrowing)
	}
	return nil
}

func (f *fakeLedger) insertEntry(userID uuid.UUID, e *model.Entry) {
	e.ID, e.EntryNo, e.CreatedAt = uuid.New(), int64(len(f.entries[userID])+1), time.Now()
	f.entries[userID] = append(f.entries[userID], e)
}

// Plans lists paid installments whose entry still exists, like ON DELETE CASCADE.
func (f *fakeLedger) Plans(_ context.Context, userID uuid.UUID, id *uuid.UUID) ([]model.Plan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.Plan{}
	for pid, p := range f.credit.plans {
		if f.credit.owner[pid] != userID || (id != nil && *id != pid) {
			continue
		}
		copied := *p
		copied.Account = *f.userAccounts(userID)[p.Account.ID]
		copied.Paid = nil
		for seq, entryID := range f.credit.payments[pid] {
			if e := f.entryByID(userID, entryID); e != nil {
				copied.Paid = append(copied.Paid, model.PaidInstallment{Seq: seq, EntryID: entryID, EntryNo: e.EntryNo})
			}
		}
		sort.Slice(copied.Paid, func(i, j int) bool { return copied.Paid[i].Seq < copied.Paid[j].Seq })
		out = append(out, copied)
	}
	return out, nil
}

func (f *fakeLedger) PayInstallment(_ context.Context, userID, planID uuid.UUID, seq int, e *model.Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if entryID, ok := f.credit.payments[planID][seq]; ok && f.entryByID(userID, entryID) != nil {
		return repository.ErrAlreadyPaid
	}
	f.insertEntry(userID, e)
	f.credit.payments[planID][seq] = e.ID
	return nil
}

func (f *fakeLedger) DeletePlan(_ context.Context, userID, planID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.credit.plans[planID]
	if !ok || f.credit.owner[planID] != userID {
		return repository.ErrPlanNotFound
	}
	for _, entryID := range f.credit.payments[planID] {
		if f.entryByID(userID, entryID) != nil {
			return repository.ErrPlanHasPayments
		}
	}
	delete(f.credit.plans, planID)
	f.userAccounts(userID)[p.Account.ID].IsActive = false
	return nil
}

func accountByCode(t *testing.T, accounts []any, code string) string {
	t.Helper()
	for _, a := range accounts {
		if a := a.(map[string]any); a["code"] == code {
			return a["id"].(string)
		}
	}
	t.Fatalf("no account %s", code)
	return ""
}

func num(v any) int64 { return int64(v.(float64)) }

func TestCardSpendingAndBill(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "card@x.com")
	today := time.Now().Format(time.DateOnly)
	openBooks(t, r, tok, time.Now().AddDate(0, -3, 0).Format(time.DateOnly))
	_, body := call(t, r, "GET", "/api/v1/accounts", tok, "")
	accounts := body["data"].([]any)
	bank, makan, admin, ewallet := accountByCode(t, accounts, "1001"), accountByCode(t, accounts, "5102"),
		accountByCode(t, accounts, "5107"), accountByCode(t, accounts, "1004")

	code, body := call(t, r, "POST", "/api/v1/cards", tok, `{"name":"BCA Visa","kind":"credit_card","statement_day":28,"due_day":15,"credit_limit":1000000000,"min_payment_pct":10}`)
	expect(t, "create card", code, 201, body)
	card := body["data"].(map[string]any)["account"].(map[string]any)
	cardID := card["id"].(string)
	if card["code"] != "2201" || card["kind"] != "credit_card" || card["type"] != "Liability" {
		t.Fatalf("card account: %v", card)
	}

	// Paying for dinner with the card: an expense now, a debt on the card.
	code, body = call(t, r, "POST", "/api/v1/entries", tok, `{"kind":"expense","entry_date":"`+today+`","amount":25000000,"from_account_id":"`+cardID+`","to_account_id":"`+makan+`"}`)
	expect(t, "expense on card", code, 201, body)
	code, body = call(t, r, "POST", "/api/v1/entries", tok, `{"kind":"income","entry_date":"`+today+`","amount":100,"from_account_id":"`+cardID+`","to_account_id":"`+bank+`"}`)
	expect(t, "a card is no income source", code, 400, body)

	_, body = call(t, r, "GET", "/api/v1/cards/"+cardID, tok, "")
	sum := body["data"].(map[string]any)
	if num(sum["owed"]) != 25000000 || num(sum["available"]) != 975000000 {
		t.Fatalf("owed/available: %v %v", sum["owed"], sum["available"])
	}

	// Paying the card from the bank, with a transfer fee in the same entry.
	code, body = call(t, r, "POST", "/api/v1/entries", tok, `{"kind":"transfer","entry_date":"`+today+`","amount":10000000,"fee":650000,"fee_account_id":"`+admin+`","from_account_id":"`+bank+`","to_account_id":"`+cardID+`"}`)
	expect(t, "pay card with fee", code, 201, body)
	lines := body["data"].(map[string]any)["lines"].([]any)
	if len(lines) != 3 || num(lines[2].(map[string]any)["credit"]) != 10650000 {
		t.Fatalf("fee lines: %v", lines)
	}
	_, body = call(t, r, "GET", "/api/v1/cards/"+cardID, tok, "")
	if owed := num(body["data"].(map[string]any)["owed"]); owed != 15000000 {
		t.Fatalf("owed after payment %d", owed)
	}
	code, body = call(t, r, "POST", "/api/v1/entries", tok, `{"kind":"expense","entry_date":"`+today+`","amount":100,"fee":5,"fee_account_id":"`+admin+`","from_account_id":"`+bank+`","to_account_id":"`+makan+`"}`)
	if code != 400 || body["code"] != "wrong_fee" {
		t.Fatalf("a fee belongs to transfers only: %d %v", code, body)
	}
	code, body = call(t, r, "POST", "/api/v1/entries", tok, `{"kind":"transfer","entry_date":"`+today+`","amount":100,"from_account_id":"`+ewallet+`","to_account_id":"`+bank+`"}`)
	expect(t, "plain transfer still works", code, 201, body)

	_, body = call(t, r, "GET", "/api/v1/reports/balance-sheet?date="+today, tok, "")
	if !contains(fmt.Sprint(body), "BCA Visa") {
		t.Fatalf("the card debt shows on the balance sheet: %v", body)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestInstallmentPlan(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "kpr@x.com")
	today := time.Now().Format(time.DateOnly)
	openBooks(t, r, tok, time.Now().AddDate(0, -1, 0).Format(time.DateOnly))
	_, body := call(t, r, "GET", "/api/v1/accounts", tok, "")
	bank := accountByCode(t, body["data"].([]any), "1001")

	// Rp1.000.000 at 12% over 12 months, received into the bank.
	code, body := call(t, r, "POST", "/api/v1/installments", tok, `{"name":"KTA","principal":100000000,"annual_rate":12,"tenor_months":12,"method":"annuity","first_due_date":"`+today+`","received_into_id":"`+bank+`"}`)
	expect(t, "create plan", code, 201, body)
	plan := body["data"].(map[string]any)
	planID := plan["id"].(string)
	if num(plan["next"].(map[string]any)["payment"]) != 8884879 || len(plan["rows"].([]any)) != 12 {
		t.Fatalf("schedule: %v", plan["next"])
	}

	pay := func() (int, map[string]any) {
		return call(t, r, "POST", "/api/v1/installments/"+planID+"/pay", tok, `{"from_account_id":"`+bank+`","entry_date":"`+today+`"}`)
	}
	code, body = pay()
	expect(t, "pay 1", code, 201, body)
	p := body["data"].(map[string]any)
	if num(p["paid_count"]) != 1 || num(p["remaining"]) != 100000000-7884879 {
		t.Fatalf("after one payment: %v %v", p["paid_count"], p["remaining"])
	}
	firstEntry := p["rows"].([]any)[0].(map[string]any)["paid"].(map[string]any)["entry_id"].(string)
	_, body = call(t, r, "GET", "/api/v1/entries/"+firstEntry, tok, "")
	lines := body["data"].(map[string]any)["entry"].(map[string]any)["lines"].([]any)
	if len(lines) != 3 || num(lines[1].(map[string]any)["debit"]) != 1000000 {
		t.Fatalf("payment splits principal and interest: %v", lines)
	}

	code, body = call(t, r, "DELETE", "/api/v1/installments/"+planID, tok, "")
	expect(t, "delete a plan with payments", code, 409, body)

	// Deleting the payment entry makes installment 1 unpaid again.
	code, body = call(t, r, "DELETE", "/api/v1/entries/"+firstEntry, tok, "")
	expect(t, "delete payment entry", code, 204, body)
	_, body = call(t, r, "GET", "/api/v1/installments/"+planID, tok, "")
	if p := body["data"].(map[string]any); num(p["paid_count"]) != 0 || num(p["next"].(map[string]any)["seq"]) != 1 {
		t.Fatalf("installment 1 should be open again: %v", p["next"])
	}

	for i := range 12 {
		code, body = pay()
		expect(t, fmt.Sprintf("pay %d", i+1), code, 201, body)
	}
	if p := body["data"].(map[string]any); p["next"] != nil || num(p["remaining"]) != 0 {
		t.Fatalf("fully paid: %v", p)
	}
	code, body = pay()
	if code != 409 || body["code"] != "already_paid" {
		t.Fatalf("a 13th payment: %d %v", code, body)
	}

	// The loan account is back to zero: received 1.000.000, repaid 1.000.000 of principal.
	_, body = call(t, r, "GET", "/api/v1/installments", tok, "")
	loan := body["data"].([]any)[0].(map[string]any)["account"].(map[string]any)["id"].(string)
	_, body = call(t, r, "GET", "/api/v1/accounts/"+loan+"/balance?date="+today, tok, "")
	if b := num(body["data"].(map[string]any)["balance"]); b != 0 {
		t.Fatalf("loan balance after the last payment: %d", b)
	}
}

func TestLoanForFixedAsset(t *testing.T) {
	r, _ := newTestServer()
	tok := signUp(t, r, "motor@x.com")
	today := time.Now().Format(time.DateOnly)
	openBooks(t, r, tok, time.Now().AddDate(0, -1, 0).Format(time.DateOnly))
	acc := accountIDs(t, r, tok)

	// A motorbike loan adds to Kendaraan, never to the bank; the bank is ignored when both are sent.
	code, body := call(t, r, "POST", "/api/v1/installments", tok, `{"name":"Kredit Motor","principal":1800000000,"annual_rate":12,"tenor_months":12,"method":"flat","first_due_date":"`+today+`","received_into_id":"`+acc["1001"]+`","asset_account_id":"`+acc["1301"]+`"}`)
	expect(t, "loan for an asset", code, 201, body)
	planID := body["data"].(map[string]any)["id"].(string)
	if body["data"].(map[string]any)["interest_account_id"] != acc["5111"] {
		t.Fatalf("loan interest defaults to 5111 Bunga Pinjaman: %v", body["data"])
	}
	_, body = call(t, r, "GET", "/api/v1/accounts/"+acc["1301"]+"/balance?date="+today, tok, "")
	if b := num(body["data"].(map[string]any)["balance"]); b != 1800000000 {
		t.Fatalf("Kendaraan balance %d", b)
	}
	_, body = call(t, r, "GET", "/api/v1/accounts/"+acc["1001"]+"/balance?date="+today, tok, "")
	if b := num(body["data"].(map[string]any)["balance"]); b != 0 {
		t.Fatalf("the bank never saw the money: %d", b)
	}
	code, body = call(t, r, "POST", "/api/v1/installments", tok, `{"name":"Salah","principal":100,"annual_rate":0,"tenor_months":1,"method":"flat","first_due_date":"`+today+`","asset_account_id":"`+acc["1001"]+`"}`)
	if code != 400 || body["code"] != "wrong_account_type" {
		t.Fatalf("a bank is no fixed asset: %d %v", code, body)
	}

	// Interest of an installment lands on Bunga Pinjaman.
	call(t, r, "POST", "/api/v1/entries", tok, `{"kind":"income","entry_date":"`+today+`","amount":500000000,"from_account_id":"`+acc["4001"]+`","to_account_id":"`+acc["1001"]+`"}`)
	code, body = call(t, r, "POST", "/api/v1/installments/"+planID+"/pay", tok, `{"from_account_id":"`+acc["1001"]+`","entry_date":"`+today+`"}`)
	expect(t, "pay installment", code, 201, body)
	entryID := body["data"].(map[string]any)["rows"].([]any)[0].(map[string]any)["paid"].(map[string]any)["entry_id"].(string)
	_, body = call(t, r, "GET", "/api/v1/entries/"+entryID, tok, "")
	if !contains(fmt.Sprint(body["data"].(map[string]any)["entry"].(map[string]any)["lines"]), "5111") {
		t.Fatalf("interest on 5111: %v", body)
	}

	// Fixed assets are no source or target for daily money.
	code, body = call(t, r, "POST", "/api/v1/entries", tok, entryBody("expense", today, 100, acc["1301"], acc["5102"]))
	expect(t, "expense from an asset", code, 400, body)
	code, body = call(t, r, "POST", "/api/v1/entries", tok, entryBody("income", today, 100, acc["4001"], acc["1301"]))
	expect(t, "income into an asset", code, 400, body)

	// Revalue down to 1.700.000.000 (Cr Kendaraan / Dr 3002), then up again; the assets
	// stay out of the dashboard balances but in the balance sheet, which stays balanced.
	code, body = call(t, r, "POST", "/api/v1/accounts/"+acc["1301"]+"/revalue", tok, `{"value":1700000000,"entry_date":"`+today+`"}`)
	expect(t, "revalue down", code, 201, body)
	lines := body["data"].(map[string]any)["lines"].([]any)
	if len(lines) != 2 || lines[0].(map[string]any)["code"] != "3002" || num(lines[0].(map[string]any)["debit"]) != 100000000 ||
		lines[1].(map[string]any)["code"] != "1301" || num(lines[1].(map[string]any)["credit"]) != 100000000 {
		t.Fatalf("revaluation lines: %v", lines)
	}
	code, body = call(t, r, "POST", "/api/v1/accounts/"+acc["1301"]+"/revalue", tok, `{"value":1700000000,"entry_date":"`+today+`"}`)
	if code != 400 || body["code"] != "no_value_change" {
		t.Fatalf("same value: %d %v", code, body)
	}
	code, body = call(t, r, "POST", "/api/v1/accounts/"+acc["1001"]+"/revalue", tok, `{"value":1,"entry_date":"`+today+`"}`)
	if code != 400 || body["code"] != "not_fixed_asset" {
		t.Fatalf("only fixed assets: %d %v", code, body)
	}
	code, body = call(t, r, "POST", "/api/v1/accounts/"+acc["1301"]+"/revalue", tok, `{"value":1750000000,"entry_date":"`+today+`"}`)
	expect(t, "revalue up", code, 201, body)
	if l := body["data"].(map[string]any)["lines"].([]any)[0].(map[string]any); l["code"] != "1301" || num(l["debit"]) != 50000000 {
		t.Fatalf("gain debits the asset: %v", l)
	}

	_, body = call(t, r, "GET", "/api/v1/reports/balance-sheet?date="+today, tok, "")
	if bs := body["data"].(map[string]any); bs["balanced"] != true || !contains(fmt.Sprint(bs["assets"]), "Kendaraan") {
		t.Fatalf("balance sheet: %v", bs)
	}
	_, body = call(t, r, "GET", "/api/v1/dashboard?date="+today, tok, "")
	if contains(fmt.Sprint(body["data"].(map[string]any)["balances"]), "Kendaraan") {
		t.Fatalf("a vehicle is not liquid money: %v", body["data"])
	}
}
