package service

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
)

type Exporter interface {
	ExportTables(ctx context.Context, userID uuid.UUID) ([]model.ExportTable, error)
}

// Export writes everything the user owns as a ZIP: data.json with every table, and one CSV
// per table (UTF-8 with BOM, so Excel shows Indonesian names correctly).
func Export(ctx context.Context, repo Exporter, userID uuid.UUID, w io.Writer) error {
	tables, err := repo.ExportTables(ctx, userID)
	if err != nil {
		return err
	}
	z := zip.NewWriter(w)

	all := map[string][]map[string]string{}
	for _, t := range tables {
		rows := make([]map[string]string, len(t.Rows))
		for i, r := range t.Rows {
			rows[i] = map[string]string{}
			for j, col := range t.Columns {
				rows[i][col] = r[j]
			}
		}
		all[t.Name] = rows
	}
	f, err := z.Create("data.json")
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(all); err != nil {
		return err
	}

	for _, t := range tables {
		f, err := z.Create(t.Name + ".csv")
		if err != nil {
			return err
		}
		if _, err := f.Write([]byte("\uFEFF")); err != nil {
			return err
		}
		c := csv.NewWriter(f)
		if err := c.Write(t.Columns); err != nil {
			return err
		}
		if err := c.WriteAll(t.Rows); err != nil { // WriteAll flushes
			return err
		}
	}
	return z.Close()
}

func (s *ReportService) Export(ctx context.Context, userID uuid.UUID, w io.Writer) error {
	return Export(ctx, s.repo, userID, w)
}
