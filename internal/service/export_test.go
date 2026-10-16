package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
)

type stubExporter []model.ExportTable

func (s stubExporter) ExportTables(context.Context, uuid.UUID) ([]model.ExportTable, error) {
	return s, nil
}

func TestExportZip(t *testing.T) {
	tables := stubExporter{
		{Name: "accounts", Columns: []string{"code", "name"}, Rows: [][]string{{"1001", "Bank, BCA"}}},
		{Name: "budgets", Columns: []string{"amount"}, Rows: [][]string{}},
	}
	var buf bytes.Buffer
	if err := Export(context.Background(), tables, uuid.New(), &buf); err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range z.File {
		r, _ := f.Open()
		b, _ := io.ReadAll(r)
		files[f.Name] = string(b)
	}
	if len(files) != 3 {
		t.Fatalf("want data.json + 2 CSVs, got %v", len(files))
	}
	var data map[string][]map[string]string
	if err := json.Unmarshal([]byte(files["data.json"]), &data); err != nil || data["accounts"][0]["name"] != "Bank, BCA" {
		t.Fatalf("data.json: %v %v", err, data)
	}
	if data["budgets"] == nil || len(data["budgets"]) != 0 {
		t.Fatalf("an empty table is an empty list, not missing: %v", data["budgets"])
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(files["accounts.csv"], "\uFEFF"))).ReadAll()
	if err != nil || len(rows) != 2 || rows[1][1] != "Bank, BCA" {
		t.Fatalf("accounts.csv: %v %v", err, rows)
	}
}
