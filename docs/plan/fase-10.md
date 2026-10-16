# Fase 10 — Insight, Grafik, dan Laporan Lengkap

Menjawab masukan A14–A19, A22, B11–B15 (lihat `masukan-dan-fase-lanjutan.md`).

## Tujuan & definisi selesai
1. **Ringkasan berlapis:** angka besar "Sisa aman per hari sampai gajian", donat pengeluaran per kategori, grafik harian, dan sampai 3 insight otomatis (naik/turun dibanding periode lalu dan rata-rata 3 periode). Di bawahnya tren kekayaan bersih dan pengeluaran per kategori 12 periode.
2. **Rasio kesehatan:** dana darurat (bulan), rasio cicilan terhadap pemasukan, saving rate rata-rata 3/6/12 periode. Masing-masing dengan penjelasan satu kalimat, ambang (baik/waspada/kurang) yang tertulis, bukan hanya warna.
3. **Arus kas** (metode langsung): kas awal + operasi + investasi + pendanaan + saldo awal = kas akhir. **Diuji:** arus kas bersih sama dengan perubahan saldo kas.
4. **Perbandingan antarperiode** di Laba Rugi: periode ini, periode lalu, rata-rata 3 periode sebelumnya.
5. **Peringatan budget** 80% dan 100% di Ringkasan dan Anggaran.
6. **Ekspor:** Excel (XLSX, dibuat server tanpa library) untuk Laba Rugi, Neraca, Arus Kas, Jurnal; PDF lewat cetak browser dengan stylesheet cetak.
7. **Ringkasan SPT Tahunan:** harta dan utang per 31 Desember, penghasilan setahun per sumber, dengan catatan bahwa ini rekap, bukan nasihat pajak.
8. **Cari jurnal berdasarkan jumlah** (tepat atau rentang).
9. Grafik dapat diakses: label teks dan nilai di setiap bagian, tabel alternatif, tidak bergantung pada warna. Angka grafik sama dengan laporan (diuji).

## Di luar cakupan
Rasio pengeluaran wajib vs gaya hidup (butuh penanda kategori wajib; Fase 11 bersama tujuan), notifikasi push untuk peringatan budget (Fase 11), PDF buatan server.

## Definisi
- **Kas:** akun Asset kecuali piutang (1101) dan kategori "Investasi". Tabungan termasuk kas (bisa diambil kapan saja).
- **Arus kas per entry:** setiap entry yang menyentuh kas dibagi ke akun lawannya: Revenue/Expense → operasi, Asset non-kas (investasi, piutang) → investasi, Liability → pendanaan, Equity (saldo awal) → saldo awal. Pindah dana antarakun kas saling meniadakan.
- **Sisa aman per hari:** jumlah sisa anggaran yang belum terpakai (kategori beranggaran) dibagi sisa hari periode termasuk hari ini. Tanpa anggaran: (pemasukan − pengeluaran) periode ini dibagi sisa hari, minimal 0.
- **Dana darurat:** kas ÷ rata-rata pengeluaran per periode dari 3 periode lengkap terakhir. ≥6 baik, 3–6 waspada, <3 kurang.
- **Rasio cicilan:** rata-rata pembayaran utang per periode (debit ke akun Liability dari entry `installment` dan `transfer`, tanpa utang kontak) ÷ rata-rata pemasukan. ≤30% baik, 30–40% waspada, >40% kurang.
- **Saving rate rata-rata:** (Σ pemasukan − Σ pengeluaran) ÷ Σ pemasukan atas 3/6/12 periode lengkap. ≥20% baik, 10–20% waspada, <10% kurang.
- **Insight:** kategori yang naik ≥20% dan ≥Rp50.000 dibanding periode lalu; kategori yang turun ≥Rp50.000 dibanding rata-rata 3 periode; perubahan saving rate ≥5 poin. Diurutkan menurut besar rupiah, maksimal 3. Dikirim sebagai data (jenis, akun, angka); teksnya dibuat frontend dalam bahasa pengguna.

## Backend
| Method | Path | Isi |
|---|---|---|
| GET | `/insights?date=` | `safe_per_day`, `by_category`, `daily`, `insights`, `budget_alerts` |
| GET | `/trends?date=&periods=12` | per periode: pemasukan, pengeluaran, kekayaan bersih di akhir periode, pengeluaran per kategori |
| GET | `/health?date=` | tiga rasio dengan nilai, ambang, dan status |
| GET | `/reports/cash-flow?date=` | arus kas periode |
| GET | `/reports/income-statement?date=&compare=1` | ditambah `prev` dan `avg3` per baris dan total |
| GET | `/reports/tax-summary?year=` | ringkasan SPT |
| GET | `/reports/export.xlsx?report=income\|balance\|cashflow\|journal&date=` | file XLSX |
| GET | `/entries?amount_min=&amount_max=` | filter jumlah (total debit entry) |

Repository baru: `DailyTotals(from, to)` (pemasukan & pengeluaran per hari) dan `EntryLines(from, to)` (baris per entry untuk arus kas). Sisanya memakai `Movements` yang sudah ada.

## Frontend
- `components/charts.tsx`: `Donut`, `Bars` (harian), `Lines` (tren) sebagai SVG buatan sendiri; setiap grafik punya `<figcaption>`, nilai tertulis di legenda, dan `<details>` "Lihat sebagai tabel".
- Ringkasan: blok atas (sisa aman/hari, insight, peringatan budget), donat + harian, lalu tren 12 periode dan kesehatan keuangan.
- Laporan: tab Laba Rugi (dengan kolom periode lalu & rata-rata), Neraca, Arus Kas, SPT; tombol "Excel" dan "Cetak / PDF".
- Transaksi: filter jumlah min/maks.
- ⓘ baru: `cashFlow`, `emergencyFund`, `debtRatio`, `safePerDay`, `netWorth`.

## Test
- `service/insights_test.go` (lewat handler): arus kas bersih = perubahan saldo kas; total donat = total beban Laba Rugi; sisa aman per hari; insight naik/turun; rasio.
- `xlsx` writer: file terbaca lagi oleh pembaca XLSX Fase 9.
- Filter jumlah jurnal.
