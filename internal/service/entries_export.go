package service

import (
	"context"
	"encoding/csv"
	"io"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/xlsx"
)

// exportLimit caps one export; the list endpoint pages at 200.
// ponytail: one query, no streaming; raise or stream if someone exports more than this.
const exportLimit = 50000

// ExportEntries writes every entry matching f (paging ignored) as CSV (UTF-8 with BOM, so
// Excel reads Indonesian text correctly) or as an Excel sheet. Filtered to one account, a
// Saldo column is added. Labels stay in Indonesian, like the report export.
func (s *LedgerService) ExportEntries(ctx context.Context, userID uuid.UUID, f model.EntryFilter, asXLSX bool, w io.Writer) error {
	f.Limit, f.Offset = exportLimit, 0
	entries, _, err := s.ListEntries(ctx, userID, f)
	if err != nil {
		return err
	}
	header := []any{"No", "Tanggal", "Jenis", "Keterangan", "Debit", "Kredit", "Jumlah"}
	if f.AccountID != nil {
		header = append(header, "Saldo")
	}
	rows := [][]any{header}
	for _, e := range entries {
		var debit, credit []string
		var amount int64
		for _, l := range e.Lines {
			if l.Debit > 0 {
				debit = append(debit, l.Name)
				amount += l.Debit
			}
			if l.Credit > 0 {
				credit = append(credit, l.Name)
			}
		}
		row := []any{e.EntryNo, e.Date, e.Kind, e.Description, strings.Join(debit, ", "), strings.Join(credit, ", "), rp(amount)}
		if f.AccountID != nil {
			if e.BalanceAfter != nil {
				row = append(row, rp(*e.BalanceAfter))
			} else {
				row = append(row, nil)
			}
		}
		rows = append(rows, row)
	}
	if asXLSX {
		return xlsx.Write(w, "Transaksi", rows)
	}

	if _, err := io.WriteString(w, "\uFEFF"); err != nil {
		return err
	}
	cells := make([][]string, len(rows))
	for i, r := range rows {
		cells[i] = make([]string, len(r))
		for j, v := range r {
			switch v := v.(type) {
			case int64:
				cells[i][j] = strconv.FormatInt(v, 10)
			case float64:
				cells[i][j] = strconv.FormatFloat(v, 'f', 2, 64)
			case string:
				cells[i][j] = v
			}
		}
	}
	return csv.NewWriter(w).WriteAll(cells) // WriteAll flushes
}
