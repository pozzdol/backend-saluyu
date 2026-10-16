package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
)

func sums(lines []model.Line) (debit, credit int64) {
	for _, l := range lines {
		debit += l.Debit
		credit += l.Credit
	}
	return
}

func TestOpeningLines(t *testing.T) {
	bank := model.Account{ID: uuid.New(), Type: "Asset"}
	cash := model.Account{ID: uuid.New(), Type: "Asset"}
	empty := model.Account{ID: uuid.New(), Type: "Asset"}
	card := model.Account{ID: uuid.New(), Type: "Liability"}
	equity := uuid.New()
	accounts := map[uuid.UUID]model.Account{bank.ID: bank, cash.ID: cash, empty.ID: empty, card.ID: card}

	// Bank 1.5jt + cash 600rb - credit card 500rb = net worth 1.6jt on the credit side of equity.
	lines := openingLines([]model.OpeningItem{
		{AccountID: bank.ID, Amount: 150000000},
		{AccountID: cash.ID, Amount: 60000000},
		{AccountID: empty.ID, Amount: 0},
		{AccountID: card.ID, Amount: 50000000},
	}, accounts, equity)
	if len(lines) != 4 {
		t.Fatalf("want 4 lines (zero balance skipped), got %d: %+v", len(lines), lines)
	}
	if last := lines[3]; last.AccountID != equity || last.Credit != 160000000 {
		t.Fatalf("equity line %+v, want credit 160000000", last)
	}
	if d, c := sums(lines); d != c {
		t.Fatalf("opening not balanced: debit %d credit %d", d, c)
	}

	// More debt than assets: equity goes to the debit side.
	lines = openingLines([]model.OpeningItem{
		{AccountID: bank.ID, Amount: 10000000},
		{AccountID: card.ID, Amount: 30000000},
	}, accounts, equity)
	if last := lines[2]; last.Debit != 20000000 {
		t.Fatalf("negative net worth line %+v, want debit 20000000", last)
	}
	if d, c := sums(lines); d != c {
		t.Fatalf("opening not balanced: debit %d credit %d", d, c)
	}

	if lines := openingLines([]model.OpeningItem{{AccountID: bank.ID}}, accounts, equity); lines != nil {
		t.Fatalf("all-zero opening should have no lines, got %+v", lines)
	}
}

func TestAdjustLines(t *testing.T) {
	cash := model.Account{ID: uuid.New(), Type: "Asset", NormalBalance: "Debit"}
	card := model.Account{ID: uuid.New(), Type: "Liability", NormalBalance: "Credit"}
	counter := uuid.New()

	tests := []struct {
		name                string
		acc                 model.Account
		diff                int64
		accDebit, accCredit int64
	}{
		{"cash missing 60rb: Dr 5199 / Cr cash", cash, -6000000, 0, 6000000},
		{"cash found 1rb: Dr cash / Cr 4199", cash, 100000, 100000, 0},
		{"card statement higher: Cr card", card, 200000, 0, 200000},
		{"card statement lower: Dr card", card, -200000, 200000, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := adjustLines(tt.acc, counter, tt.diff)
			if len(lines) != 2 || lines[0].Debit != tt.accDebit || lines[0].Credit != tt.accCredit {
				t.Fatalf("got %+v", lines)
			}
			if d, c := sums(lines); d != c {
				t.Fatalf("not balanced: %d vs %d", d, c)
			}
		})
	}
	if adjustLines(cash, counter, 0) != nil {
		t.Fatal("diff 0 must post nothing")
	}
}

func TestRupiah(t *testing.T) {
	for sen, want := range map[int64]string{0: "Rp0", 900000: "Rp9.000", 1082601292: "Rp10.826.012,92", -5: "-Rp0,05"} {
		if got := rupiah(sen); got != want {
			t.Errorf("rupiah(%d) = %q, want %q", sen, got, want)
		}
	}
}
