package service

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/xlsx"
)

var ErrUnknownReport = errors.New("report must be income, balance, cashflow or journal")

func rp(sen int64) float64 { return float64(sen) / 100 }

func section(rows *[][]any, title string, items []model.AmountRow, total int64) {
	*rows = append(*rows, []any{title})
	for _, r := range items {
		*rows = append(*rows, []any{r.Code, r.Name, rp(r.Amount)})
	}
	*rows = append(*rows, []any{nil, "Total " + title, rp(total)}, []any{})
}

// ExportReport writes one report of the period containing date as an Excel sheet.
// Labels stay in Indonesian, like the account names they sit next to.
func (s *ReportService) ExportReport(ctx context.Context, userID uuid.UUID, report string, date time.Time, w io.Writer) (string, error) {
	var rows [][]any
	var name string
	switch report {
	case "income":
		is, err := s.IncomeStatement(ctx, userID, date, false)
		if err != nil {
			return "", err
		}
		name = "Laba Rugi " + is.Period.Start
		rows = [][]any{{"Laba Rugi", is.Period.Label}, {}}
		section(&rows, "Pendapatan", is.Revenue, is.RevenueTotal)
		section(&rows, "Beban", is.Expense, is.ExpenseTotal)
		rows = append(rows, []any{nil, "Laba (rugi) bersih", rp(is.NetIncome)})
	case "balance":
		bs, err := s.BalanceSheet(ctx, userID, date)
		if err != nil {
			return "", err
		}
		name = "Neraca " + bs.AsOf
		rows = [][]any{{"Neraca", bs.AsOf}, {}}
		section(&rows, "Aset", bs.Assets, bs.AssetsTotal)
		section(&rows, "Liabilitas", bs.Liabilities, bs.LiabilitiesTotal)
		section(&rows, "Ekuitas", bs.Equity, bs.EquityTotal)
		rows = append(rows, []any{nil, "Laba berjalan", rp(bs.CurrentEarnings)})
	case "cashflow":
		cf, err := s.CashFlow(ctx, userID, date)
		if err != nil {
			return "", err
		}
		name = "Arus Kas " + cf.Period.Start
		rows = [][]any{{"Arus Kas", cf.Period.Label}, {}, {nil, "Kas awal", rp(cf.Beginning)}, {}}
		section(&rows, "Operasi", cf.Operating.Rows, cf.Operating.Total)
		section(&rows, "Investasi", cf.Investing.Rows, cf.Investing.Total)
		section(&rows, "Pendanaan", cf.Financing.Rows, cf.Financing.Total)
		rows = append(rows, []any{nil, "Saldo awal dicatat", rp(cf.Opening)}, []any{nil, "Arus kas bersih", rp(cf.Net)}, []any{nil, "Kas akhir", rp(cf.Ending)})
	case "journal":
		p, err := s.periodOf(ctx, userID, date)
		if err != nil {
			return "", err
		}
		lines, err := s.repo.EntryLines(ctx, userID, p.Start, p.Next)
		if err != nil {
			return "", err
		}
		name = "Jurnal " + p.Start.Format(time.DateOnly)
		rows = [][]any{{"No", "Tanggal", "Jenis", "Keterangan", "Kode", "Akun", "Debit", "Kredit"}}
		for _, l := range lines {
			rows = append(rows, []any{l.EntryNo, l.Date.Format(time.DateOnly), l.Kind, l.Description, l.Code, l.Name, rp(l.Debit), rp(l.Credit)})
		}
	default:
		return "", ErrUnknownReport
	}
	return name, xlsx.Write(w, name[:min(len(name), 31)], rows)
}
