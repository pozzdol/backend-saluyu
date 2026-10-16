package statement

import (
	"archive/zip"
	"bytes"
	"testing"
	"time"
)

var today = time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)

func TestParseAmount(t *testing.T) {
	for _, c := range []struct {
		in, dec string
		want    int64
	}{
		{"1.234.567,89", "comma", 123456789},
		{"1,234,567.89", "dot", 123456789},
		{"-50.000", "comma", -5000000},
		{"(50.000)", "comma", -5000000},
		{"50,000.00 DB", "dot", -5000000},
		{"Rp 75.000 CR", "comma", 7500000},
		{"", "comma", 0},
		{"10.000", "comma", 1000000},
	} {
		got, err := ParseAmount(c.in, c.dec)
		if err != nil || got != c.want {
			t.Errorf("%q: got %d %v want %d", c.in, got, err, c.want)
		}
	}
	if _, err := ParseAmount("abc", "comma"); err == nil {
		t.Error("letters are not an amount")
	}
}

func TestParseDate(t *testing.T) {
	for _, c := range []struct{ in, format, want string }{
		{"25/10/2026", "dmy", "2026-10-25"},
		{"10/25/2026", "mdy", "2026-10-25"},
		{"2026-10-25 13:05", "ymd", "2026-10-25"},
		{"'25/09", "dmy", "2026-09-25"}, // BCA: no year, this year
		{"'25/12", "dmy", "2025-12-25"}, // later month than today: last year
		{"5 Okt 2026", "dmy", "2026-10-05"},
		{"05-Dec-25", "dmy", "2025-12-05"},
		{"46311", "dmy", "2026-10-16"}, // Excel serial
	} {
		got, ok := ParseDate(c.in, c.format, today)
		if !ok || got.Format(time.DateOnly) != c.want {
			t.Errorf("%q: got %v %v want %s", c.in, got, ok, c.want)
		}
	}
	if _, ok := ParseDate("31/02/2026", "dmy", today); ok {
		t.Error("31 February is no date")
	}
}

func TestCSVDetectAndParse(t *testing.T) {
	// A BCA-like export (the CSV reader drops the blank line): title lines first, ';', BOM, one Mutasi column with DB/CR.
	file := "\xef\xbb\xbfInformasi Rekening;;;\nNo. rekening;123;;\n\n" +
		"Tanggal Transaksi;Keterangan;Cabang;Mutasi;Saldo\n" +
		"'01/10;TRSF E-BANKING DB GRAB*FOOD;0000;45.000,00 DB;955.000,00\n" +
		"'01/10;TRSF E-BANKING DB GRAB*FOOD;0000;45.000,00 DB;910.000,00\n" +
		"'02/10;GAJI OKTOBER;0000;10.000.000,00 CR;10.910.000,00\n" +
		"Saldo Akhir;;;;10.910.000,00\n"
	cells, err := Read([]byte(file))
	if err != nil {
		t.Fatal(err)
	}
	m, err := Detect(cells)
	if err != nil {
		t.Fatal(err)
	}
	if m.HeaderRow != 2 || m.DateCol != 0 || m.AmountCol == nil || *m.AmountCol != 3 || m.Decimal != "comma" || m.DateFormat != "dmy" {
		t.Fatalf("mapping %+v", m)
	}
	rows, errs := Parse(cells, m, today)
	if len(errs) != 0 || len(rows) != 3 {
		t.Fatalf("rows %+v errs %+v", rows, errs)
	}
	if rows[0].Amount != -4500000 || rows[2].Amount != 1000000000 || rows[0].Date != "2026-10-01" || *rows[2].Balance != 1091000000 {
		t.Fatalf("%+v", rows)
	}
}

func TestDebitCreditColumns(t *testing.T) {
	file := "Date,Description,Money Out,Money In,Balance\n10/03/2026,Coffee,\"1,500.00\",,98.50\n10/04/2026,Refund,,20.00,118.50\n"
	cells, _ := Read([]byte(file))
	m, err := Detect(cells)
	if err != nil {
		t.Fatal(err)
	}
	if m.AmountCol != nil || m.DebitCol == nil || m.CreditCol == nil || m.Decimal != "dot" {
		t.Fatalf("mapping %+v", m)
	}
	rows, _ := Parse(cells, m, today)
	if len(rows) != 2 || rows[0].Amount != -150000 || rows[1].Amount != 2000 {
		t.Fatalf("%+v", rows)
	}
}

func TestXLSX(t *testing.T) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, _ := z.Create("xl/sharedStrings.xml")
	w.Write([]byte(`<sst><si><t>Tanggal</t></si><si><t>Keterangan</t></si><si><t>Jumlah</t></si><si><r><t>Kopi </t></r><r><t>Kenangan</t></r></si></sst>`))
	w, _ = z.Create("xl/worksheets/sheet1.xml")
	w.Write([]byte(`<worksheet><sheetData>
		<row><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="s"><v>2</v></c></row>
		<row><c r="A2"><v>46311</v></c><c r="B2" t="s"><v>3</v></c><c r="C2"><v>-25000</v></c></row>
	</sheetData></worksheet>`))
	z.Close()
	cells, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	m, err := Detect(cells)
	if err != nil {
		t.Fatal(err)
	}
	m.Decimal = "dot" // XLSX numbers are plain
	rows, _ := Parse(cells, m, today)
	if len(rows) != 1 || rows[0].Date != "2026-10-16" || rows[0].Description != "Kopi Kenangan" || rows[0].Amount != -2500000 {
		t.Fatalf("%+v", rows)
	}
}

func TestMappingFits(t *testing.T) {
	a, _ := Read([]byte("Tanggal;Keterangan;Mutasi\n01/10/2026;X;1,00\n"))
	b, _ := Read([]byte("Tanggal,Keterangan,Debit,Kredit\n01/10/2026,X,1,\n"))
	m, _ := Detect(a)
	if !m.Fits(a) || m.Fits(b) {
		t.Fatal("a mapping fits only the layout it was made for")
	}
}
