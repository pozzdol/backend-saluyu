// Package statement reads bank and e-wallet statement exports (CSV or XLSX) into rows of
// date, description and signed amount. Columns are found from the header words, because
// every bank names and orders them differently and changes its export now and then.
package statement

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Mapping says which column holds what. Column numbers start at 0.
type Mapping struct {
	HeaderRow       int    `json:"header_row"` // data starts on the next row
	DateCol         int    `json:"date_col"`
	DescriptionCols []int  `json:"description_cols"`
	AmountCol       *int   `json:"amount_col"` // one signed column (or with DB/CR), or
	DebitCol        *int   `json:"debit_col"`  // money out and
	CreditCol       *int   `json:"credit_col"` // money in as two columns
	BalanceCol      *int   `json:"balance_col"`
	DateFormat      string `json:"date_format"` // dmy, mdy or ymd
	Decimal         string `json:"decimal"`     // comma (1.234,56) or dot (1,234.56)
	Negate          bool   `json:"negate"`      // flip signs, e.g. a card statement listing purchases as positive
	// Headers is the header row the mapping was made for. A saved mapping is only reused
	// for a file with the same headers; another layout is detected afresh.
	Headers []string `json:"headers"`
}

// Fits reports whether the mapping was made for a file laid out like cells.
func (m Mapping) Fits(cells [][]string) bool {
	if m.HeaderRow < 0 || m.HeaderRow >= len(cells) || len(m.Headers) == 0 {
		return false
	}
	got := cells[m.HeaderRow]
	if len(got) != len(m.Headers) {
		return false
	}
	for i := range got {
		if !strings.EqualFold(strings.TrimSpace(got[i]), strings.TrimSpace(m.Headers[i])) {
			return false
		}
	}
	return true
}

type Row struct {
	Line        int    `json:"line"` // 1-based row among the non-empty rows of the file
	Date        string `json:"date"`
	Description string `json:"description"`
	Amount      int64  `json:"amount"` // sen; positive = money in
	Balance     *int64 `json:"balance"`
}

type LineError struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

var ErrUnreadable = errors.New("cannot read this file as CSV or XLSX")

// Read returns the cells of a CSV file or of the first sheet of an XLSX file.
func Read(data []byte) ([][]string, error) {
	if bytes.HasPrefix(data, []byte("PK")) {
		return readXLSX(data)
	}
	return readCSV(data)
}

func readCSV(data []byte) ([][]string, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	head := string(data[:min(len(data), 2000)])
	sep := ','
	for _, c := range []rune{';', '\t', '|'} {
		if strings.Count(head, string(c)) > strings.Count(head, string(sep)) {
			sep = c
		}
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma, r.FieldsPerRecord, r.LazyQuotes, r.TrimLeadingSpace = sep, -1, true, true
	cells, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	return cells, nil
}

func readXLSX(data []byte) ([][]string, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	open := func(name string) ([]byte, error) {
		for _, f := range z.File {
			if f.Name == name {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(io.LimitReader(rc, 50<<20))
			}
		}
		return nil, nil
	}
	var shared []string
	if b, err := open("xl/sharedStrings.xml"); err != nil {
		return nil, err
	} else if b != nil {
		var sst struct {
			SI []struct {
				T string `xml:"t"`
				R []struct {
					T string `xml:"t"`
				} `xml:"r"`
			} `xml:"si"`
		}
		if err := xml.Unmarshal(b, &sst); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
		}
		for _, si := range sst.SI {
			s := si.T
			for _, r := range si.R {
				s += r.T
			}
			shared = append(shared, s)
		}
	}
	b, err := open("xl/worksheets/sheet1.xml")
	if err != nil || b == nil {
		return nil, fmt.Errorf("%w: no first sheet", ErrUnreadable)
	}
	var sheet struct {
		Rows []struct {
			Cells []struct {
				Ref    string `xml:"r,attr"`
				Type   string `xml:"t,attr"`
				Value  string `xml:"v"`
				Inline string `xml:"is>t"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(b, &sheet); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	out := make([][]string, 0, len(sheet.Rows))
	for _, row := range sheet.Rows {
		var cells []string
		for _, c := range row.Cells {
			col := colIndex(c.Ref)
			for len(cells) <= col {
				cells = append(cells, "")
			}
			v := c.Value
			switch c.Type {
			case "s":
				if i, err := strconv.Atoi(v); err == nil && i < len(shared) {
					v = shared[i]
				}
			case "inlineStr":
				v = c.Inline
			}
			cells[col] = v
		}
		out = append(out, cells)
	}
	return out, nil
}

// colIndex turns "C7" into 2.
func colIndex(ref string) int {
	n := 0
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		n = n*26 + int(r-'A'+1)
	}
	return max(n-1, 0)
}

var headerWords = map[string][]string{
	"date":        {"tanggal", "tgl", "date", "waktu", "time"},
	"description": {"keterangan", "deskripsi", "description", "remark", "uraian", "catatan", "detail", "berita", "merchant", "note"},
	"debit":       {"debit", "debet", "keluar", "withdrawal", "pengeluaran", "money out"},
	"credit":      {"kredit", "credit", "masuk", "deposit", "pemasukan", "money in"},
	"amount":      {"jumlah", "amount", "nominal", "mutasi", "nilai"},
	"balance":     {"saldo", "balance"},
}

func headerKind(cell string) string {
	c := strings.ToLower(strings.TrimSpace(cell))
	for _, kind := range []string{"date", "balance", "debit", "credit", "amount", "description"} {
		for _, w := range headerWords[kind] {
			if strings.Contains(c, w) {
				return kind
			}
		}
	}
	return ""
}

// Detect finds the header row (the one with the most known words in the first 20 rows)
// and guesses the date and number formats from the data under it.
func Detect(cells [][]string) (Mapping, error) {
	best, bestHits := -1, 1
	for i, row := range cells[:min(len(cells), 20)] {
		hits := 0
		for _, c := range row {
			if headerKind(c) != "" {
				hits++
			}
		}
		if hits > bestHits {
			best, bestHits = i, hits
		}
	}
	if best < 0 {
		return Mapping{}, errors.New("no header row with date and amount columns found")
	}
	m := Mapping{HeaderRow: best, DateCol: -1, Headers: cells[best]}
	for j, c := range cells[best] {
		col := j
		switch headerKind(c) {
		case "date":
			if m.DateCol < 0 {
				m.DateCol = col
			}
		case "description":
			m.DescriptionCols = append(m.DescriptionCols, col)
		case "debit":
			m.DebitCol = &col
		case "credit":
			m.CreditCol = &col
		case "amount":
			m.AmountCol = &col
		case "balance":
			m.BalanceCol = &col
		}
	}
	if m.DebitCol != nil && m.CreditCol != nil {
		m.AmountCol = nil
	}
	if m.DateCol < 0 || (m.AmountCol == nil && (m.DebitCol == nil || m.CreditCol == nil)) {
		return m, errors.New("found a header row, but not both a date and an amount column")
	}
	data := cells[best+1:]
	m.DateFormat = guessDateFormat(data, m.DateCol)
	m.Decimal = guessDecimal(data, m)
	return m, nil
}

// monthName matches "25 Oct 2026", "25-Okt-26", "5 Desember 2026".
var monthName = regexp.MustCompile(`(\d{1,2})[ -]([A-Za-z]{3,})[ -](\d{2,4})`)

var months = map[string]time.Month{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "mei": 5, "jun": 6, "jul": 7, "aug": 8, "agu": 8, "ags": 8,
	"sep": 9, "oct": 10, "okt": 10, "nov": 11, "dec": 12, "des": 12,
}

var dateParts = regexp.MustCompile(`(\d+)\D+(\d+)(?:\D+(\d+))?`)

func guessDateFormat(rows [][]string, col int) string {
	format := "dmy" // Indonesian banks write day first
	for _, r := range rows {
		if col >= len(r) {
			continue
		}
		p := dateParts.FindStringSubmatch(r[col])
		if p == nil {
			continue
		}
		a, _ := strconv.Atoi(p[1])
		b, _ := strconv.Atoi(p[2])
		switch {
		case len(p[1]) == 4:
			return "ymd"
		case a > 12:
			return "dmy"
		case b > 12:
			format = "mdy"
		}
	}
	return format
}

var commaCents, dotCents = regexp.MustCompile(`\d,\d{1,2}\b`), regexp.MustCompile(`\d\.\d{1,2}\b`)

func guessDecimal(rows [][]string, m Mapping) string {
	comma, dot := 0, 0
	for _, r := range rows {
		for _, c := range []*int{m.AmountCol, m.DebitCol, m.CreditCol, m.BalanceCol} {
			if c == nil || *c >= len(r) {
				continue
			}
			v := strings.TrimSpace(r[*c])
			if commaCents.MatchString(v) && !dotCents.MatchString(v) {
				comma++
			} else if dotCents.MatchString(v) {
				dot++
			}
		}
	}
	if dot > comma {
		return "dot"
	}
	return "comma"
}

// Parse turns the data rows into statement rows. Rows that are not transactions (empty,
// totals, a second header) are skipped silently when they have no date; rows with a date
// but an unreadable amount are reported.
func Parse(cells [][]string, m Mapping, today time.Time) ([]Row, []LineError) {
	var rows []Row
	var errs []LineError
	cell := func(r []string, c *int) string {
		if c == nil || *c < 0 || *c >= len(r) {
			return ""
		}
		return strings.TrimSpace(r[*c])
	}
	for i := m.HeaderRow + 1; i < len(cells); i++ {
		r, line := cells[i], i+1
		date, ok := ParseDate(cell(r, &m.DateCol), m.DateFormat, today)
		if !ok {
			continue
		}
		var amount int64
		var err error
		if m.AmountCol != nil {
			amount, err = ParseAmount(cell(r, m.AmountCol), m.Decimal)
		} else {
			var out, in int64
			if out, err = ParseAmount(cell(r, m.DebitCol), m.Decimal); err == nil {
				if in, err = ParseAmount(cell(r, m.CreditCol), m.Decimal); err == nil {
					amount = abs(in) - abs(out)
				}
			}
		}
		if err != nil {
			errs = append(errs, LineError{Line: line, Message: err.Error()})
			continue
		}
		if amount == 0 {
			continue
		}
		if m.Negate {
			amount = -amount
		}
		var parts []string
		for _, c := range m.DescriptionCols {
			if v := cell(r, &c); v != "" {
				parts = append(parts, v)
			}
		}
		row := Row{Line: line, Date: date.Format(time.DateOnly), Description: strings.Join(strings.Fields(strings.Join(parts, " ")), " "), Amount: amount}
		if b, err := ParseAmount(cell(r, m.BalanceCol), m.Decimal); err == nil && cell(r, m.BalanceCol) != "" {
			row.Balance = &b
		}
		rows = append(rows, row)
	}
	return rows, errs
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// ParseDate reads 25/10/2026, 2026-10-25, 25-Oct-2026, an Excel serial day number, or a
// date without a year (BCA's '25/10), which is taken as the latest such date not after today.
func ParseDate(s, format string, today time.Time) (time.Time, bool) {
	s = strings.Trim(strings.TrimSpace(s), "'")
	if s == "" {
		return time.Time{}, false
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil && n > 20000 && n < 80000 {
		return time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int(n)), true
	}
	if p := monthName.FindStringSubmatch(s); p != nil {
		if mo, ok := months[strings.ToLower(p[2][:3])]; ok {
			d, _ := strconv.Atoi(p[1])
			y, _ := strconv.Atoi(p[3])
			if y < 100 {
				y += 2000
			}
			t := time.Date(y, mo, d, 0, 0, 0, 0, time.UTC)
			return t, t.Day() == d
		}
	}
	p := dateParts.FindStringSubmatch(s)
	if p == nil {
		return time.Time{}, false
	}
	a, _ := strconv.Atoi(p[1])
	b, _ := strconv.Atoi(p[2])
	c, _ := strconv.Atoi(p[3])
	var y, mo, d int
	switch format {
	case "ymd":
		y, mo, d = a, b, c
	case "mdy":
		mo, d, y = a, b, c
	default:
		d, mo, y = a, b, c
	}
	if p[3] == "" { // no year
		y = today.Year()
		if time.Month(mo) > today.Month() {
			y--
		}
	} else if y < 100 {
		y += 2000
	}
	if mo < 1 || mo > 12 || d < 1 || d > 31 {
		return time.Time{}, false
	}
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	if t.Day() != d {
		return time.Time{}, false // 31 February
	}
	return t, true
}

// ParseAmount reads "1.234.567,89", "1,234,567.89", "-50.000", "(50.000)", "50.000 DB",
// "Rp 50.000 CR" into signed sen. DB/D, a minus sign or parentheses mean money out.
// An empty cell is 0.
func ParseAmount(s, decimal string) (int64, error) {
	v := strings.ToUpper(strings.TrimSpace(s))
	if v == "" || v == "-" {
		return 0, nil
	}
	neg := false
	for _, suf := range []string{"DB", "CR", "D", "K", "C"} {
		if strings.HasSuffix(v, suf) && !strings.HasSuffix(v, "IDR") {
			neg = suf == "DB" || suf == "D"
			v = strings.TrimSpace(strings.TrimSuffix(v, suf))
			break
		}
	}
	v = strings.NewReplacer("RP", "", "IDR", "", " ", "", " ", "").Replace(v)
	if strings.HasPrefix(v, "(") && strings.HasSuffix(v, ")") {
		neg, v = true, strings.Trim(v, "()")
	}
	if strings.HasPrefix(v, "-") {
		neg, v = !neg, v[1:]
	}
	v = strings.TrimPrefix(v, "+")
	if decimal == "dot" {
		v = strings.ReplaceAll(v, ",", "")
	} else {
		v = strings.ReplaceAll(strings.ReplaceAll(v, ".", ""), ",", ".")
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsInf(f, 0) || f > 1e15 {
		return 0, fmt.Errorf("unreadable amount %q", s)
	}
	n := int64(math.Round(f * 100))
	if neg {
		n = -n
	}
	return n, nil
}
