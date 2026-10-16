package xlsx

import (
	"bytes"
	"testing"

	"github.com/pozzdol/backend-saluyu/internal/statement"
)

// What we write, the statement reader of phase 9 reads back.
func TestRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, "Laba Rugi", [][]any{{"Tanggal", "Keterangan", "Jumlah"}, {"2026-10-01", "Kopi & <roti>", -25000.5}, {nil, "Gaji", int64(10000000)}})
	if err != nil {
		t.Fatal(err)
	}
	cells, err := statement.Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(cells) != 3 || cells[1][1] != "Kopi & <roti>" || cells[1][2] != "-25000.5" || cells[2][2] != "10000000" || cells[2][0] != "" {
		t.Fatalf("%q", cells)
	}
	if col(0) != "A" || col(25) != "Z" || col(26) != "AA" {
		t.Fatal("column letters")
	}
}
