package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
)

func TestCheckDraft(t *testing.T) {
	bca := model.Account{ID: uuid.New(), Code: "1002", Name: "BCA", Type: "Asset", IsActive: true}
	makan := model.Account{ID: uuid.New(), Code: "5101", Name: "Makan", Type: "Expense", IsActive: true}
	lida := model.Contact{ID: uuid.New(), Name: "Lida", Payable: 100_000_00} // I owe Lida Rp100.000
	accounts, contacts := []model.Account{bca, makan}, []model.Contact{lida}

	expense := func() model.DraftItem {
		return model.DraftItem{Target: "entry", Kind: "expense", Amount: 54000, EntryDate: "2026-10-30", From: bca.ID.String(), To: makan.ID.String()}
	}
	repay := func(amount float64, confirm bool) model.DraftItem {
		return model.DraftItem{Target: "loan", LoanType: "lunas_keluar", Amount: amount, EntryDate: "2026-10-30",
			Account: bca.ID.String(), Contact: lida.ID.String(), ConfirmOverpay: confirm}
	}
	cases := []struct {
		name string
		r    model.AssistantReply
		save bool
	}{
		{"complete expense", model.AssistantReply{Save: true, Items: []model.DraftItem{expense()}}, true},
		{"user did not ask to save", model.AssistantReply{Items: []model.DraftItem{expense()}}, false},
		{"no items", model.AssistantReply{Save: true}, false},
		{"made-up id", model.AssistantReply{Save: true, Items: []model.DraftItem{func() model.DraftItem { i := expense(); i.To = uuid.NewString(); return i }()}}, false},
		{"new category the user agreed to", model.AssistantReply{Save: true,
			Items:       []model.DraftItem{func() model.DraftItem { i := expense(); i.To = "new:Parkir"; return i }()},
			NewAccounts: []model.NewAccount{{Name: "Parkir", Type: "Expense", Icon: "car"}}}, true},
		{"new: without its account", model.AssistantReply{Save: true, Items: []model.DraftItem{func() model.DraftItem { i := expense(); i.To = "new:Parkir"; return i }()}}, false},
		{"repay what is owed", model.AssistantReply{Save: true, Items: []model.DraftItem{repay(100000, false)}}, true},
		{"overpay unconfirmed", model.AssistantReply{Save: true, Items: []model.DraftItem{repay(200000, false)}}, false},
		{"overpay confirmed", model.AssistantReply{Save: true, Items: []model.DraftItem{repay(200000, true)}}, true},
		{"names instead of ids, loan type in kind", model.AssistantReply{Save: true, Items: []model.DraftItem{{Target: "loan", Kind: "lunas_keluar",
			Amount: 100000, EntryDate: "2026-10-30", Account: "bca", Contact: "Lida"}}}, true},
		{"repay a new person", model.AssistantReply{Save: true, Items: []model.DraftItem{func() model.DraftItem { i := repay(5000, false); i.Contact = "new:Budi"; return i }()}}, false},
	}
	for _, c := range cases {
		checkDraft(&c.r, "2026-10-30", accounts, contacts)
		if c.r.Save != c.save {
			t.Errorf("%s: save = %v, want %v", c.name, c.r.Save, c.save)
		}
	}

	r := model.AssistantReply{Items: []model.DraftItem{{Target: "entry", From: uuid.NewString(), EntryDate: "kemarin"}}}
	checkDraft(&r, "2026-10-30", accounts, contacts)
	if r.Items[0].From != "" || r.Items[0].EntryDate != "2026-10-30" {
		t.Errorf("unknown id and bad date should be cleared, got %+v", r.Items[0])
	}
	empty := model.AssistantReply{}
	if checkDraft(&empty, "2026-10-30", accounts, contacts); empty.Items == nil || empty.NewAccounts == nil {
		t.Error("lists should never be null in the reply")
	}
	if len(accountIcons) < 10 || accountIcons[0] != "wallet" {
		t.Errorf("accountIcons not read from the binding tag: %v", accountIcons)
	}
}
