# Fase 3 — Periode Gajian, Budget & Dashboard

## Tujuan & definisi selesai (testable)
- `period.For(2026-10-16, 25)` → 25 Sep–24 Okt 2026; semua aggregate dashboard pakai range ini (bug Excel "Oktober defisit palsu" hilang).
- `GET /dashboard` menampilkan income = semua akun Revenue (Gaji + Bunga), expense = semua akun Expense (tidak ada baris terlewat), surplus, savings rate, transfer ke tabungan/investasi terpisah (bukan expense), budget vs actual per `account_id` (bukan nama), saldo aset per akhir periode.
- Budget periode baru otomatis "mewarisi" budget terakhir tanpa perlu disalin manual.
- Laba Rugi per periode dan Neraca per tanggal; Neraca selalu `assets = liabilities + equity + current_earnings` (`balanced: true`).
- Semua angka dihitung dari `journal_lines` lewat SQL, tidak ada kolom saldo tersimpan.
- 3 test hijau: period table test, dashboard aggregate, balance-sheet equation.

## Di luar cakupan
Refund/koreksi expense (Fase 4), grup/summary (Fase 5), export, grafik tren multi-periode, hapus budget (set 0 saja), cache hasil aggregate.

## Database
**000007_add_period_settings.up.sql**
```sql
ALTER TABLE users ADD COLUMN period_start_day SMALLINT NOT NULL DEFAULT 25
  CHECK (period_start_day BETWEEN 1 AND 28); -- 28: ada di semua bulan, tak perlu clamp
-- Tanggal yang dipakai laporan periode. NULL = entry_date. Diisi saat user mencentang
-- "untuk periode berikutnya" (gaji masuk lebih awal karena libur); entry_date tetap tanggal asli.
ALTER TABLE journal_entries ADD COLUMN period_date DATE;
```
down: `ALTER TABLE journal_entries DROP COLUMN period_date; ALTER TABLE users DROP COLUMN period_start_day;`

**000008_create_budgets_table.up.sql**
```sql
CREATE TABLE budgets (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  account_id   UUID NOT NULL REFERENCES accounts(id),
  period_start DATE NOT NULL,
  amount       BIGINT NOT NULL CHECK (amount >= 0), -- sen
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (user_id, account_id, period_start)
);
-- Untuk query DISTINCT ON "budget terakhir <= periode ini"
CREATE INDEX budgets_lookup_idx ON budgets (user_id, account_id, period_start DESC);
-- journal_entries_user_date_idx dan journal_lines_account_idx sudah dibuat Fase 1.
CREATE INDEX IF NOT EXISTS journal_lines_entry_idx ON journal_lines (entry_id);
```
down: `DROP TABLE budgets; DROP INDEX IF EXISTS journal_lines_entry_idx;`

## Backend
Files baru: `internal/period/period.go` (+`_test.go`), `internal/model/report.go`, `internal/repository/report.go`, `internal/service/report.go`, `internal/handler/report.go` (+`_test.go`). Diubah: `routes.go`, `main.go`, `fail()`, `model/user.go` + `repository/user.go` (field `period_start_day`).

**Period function** (`internal/period/period.go`)
```go
type Period struct{ Start, End, Next time.Time } // End inklusif (tampilan), Next = End+1 (SQL half-open)
func For(d time.Time, startDay int) Period {
    y, m, _ := d.Date()
    if d.Day() < startDay { m-- }              // time.Date menormalkan month 0 → Des tahun lalu
    start := time.Date(y, m, startDay, 0, 0, 0, 0, time.UTC)
    next := start.AddDate(0, 1, 0)
    return Period{start, next.AddDate(0, 0, -1), next}
}
func (p Period) Label() string // "25 Sep – 24 Okt 2026"
```
Ganti start day mid-way: fungsi murni memakai nilai `users.period_start_day` saat ini; histori ikut ter-slice ulang. Budget lama tetap ditemukan karena lookup `period_start <= start` (bukan `=`).

**Satu query inti** (`repository/report.go`), dipakai dashboard, Laba Rugi, Neraca:
```sql
-- Movements(ctx, userID, from, to): mutasi per akun per kind dalam [from, to)
SELECT a.id, a.name, a.type, a.category, e.kind,
       COALESCE(SUM(l.debit),0), COALESCE(SUM(l.credit),0)
FROM journal_lines l
JOIN journal_entries e ON e.id = l.entry_id
JOIN accounts a ON a.id = l.account_id
WHERE e.user_id = $1 AND COALESCE(e.period_date, e.entry_date) >= $2
  AND COALESCE(e.period_date, e.entry_date) < $3
GROUP BY a.id, a.name, a.type, a.category, e.kind
```
Laba Rugi dan dashboard memakai `COALESCE(period_date, entry_date)`. Neraca dan saldo akun memakai `entry_date` asli (uang memang sudah ada di bank tanggal 24), jadi Neraca punya query sendiri dengan filter `entry_date`: `Movements(user, '0001-01-01', date+1)` versi `entry_date`. Saldo aset akhir periode = sama dengan `to = period.Next`. Penjumlahan per tipe dilakukan di Go (`map[uuid.UUID]*row`), bukan query terpisah.

```sql
-- EffectiveBudgets: budget terakhir per akun yang berlaku untuk periode ini (= "copy forward" tanpa write)
SELECT DISTINCT ON (b.account_id) b.account_id, a.name, b.period_start, b.amount
FROM budgets b JOIN accounts a ON a.id = b.account_id
WHERE b.user_id = $1 AND b.period_start <= $2
ORDER BY b.account_id, b.period_start DESC;

-- UpsertBudget
INSERT INTO budgets (user_id, account_id, period_start, amount) VALUES ($1,$2,$3,$4)
ON CONFLICT (user_id, account_id, period_start) DO UPDATE SET amount = EXCLUDED.amount, updated_at = now();
-- sebelumnya: SELECT type FROM accounts WHERE id=$2 AND user_id=$1 → ErrAccountNotFound / ErrNotExpenseAccount
```

**Interfaces di service** (fake di test): `LedgerReader{ Movements(...) }`, `BudgetRepository{ Effective, Upsert }`. `ReportService` juga butuh `UserRepository.FindByID` untuk `period_start_day`. Fungsi summary per user (`ForUser(userID, date)`) dipakai ulang Fase 5.

**Endpoints** (semua di balik `Auth`, `?date=YYYY-MM-DD` default hari ini, tanggal apa pun di dalam periode):
- `GET /api/v1/dashboard?date=` →
  `{"period":{"start","end","next","prev","label"},"income","expense","surplus","savings_rate":0.32,"to_savings":[{"account_id","name","amount"}],"to_savings_total","budgets":[{"account_id","name","budget","actual","remaining","over":bool,"inherited":bool}],"balances":[{"account_id","name","category","balance"}]}`
  income = Σ Revenue (credit−debit); expense = Σ Expense (debit−credit); to_savings = debit akun Asset dengan `category IN ('Tabungan','Investasi')` pada kind=`transfer`; savings_rate = surplus/income (0 jika income 0); `prev = start−1 hari` (untuk tombol navigasi).
- `GET /api/v1/budgets?date=` → `{"period","data":[budgets seperti di atas]}`; `PUT /api/v1/budgets/:account_id` body `{"date":"2026-10-16","amount":150000000}` (server snap ke `period.Start`) → 200 row.
- `GET /api/v1/reports/income-statement?date=` → `{"period","revenue":[{account_id,name,amount}],"revenue_total","expense":[...],"expense_total","net_income"}`
- `GET /api/v1/reports/balance-sheet?date=` → `{"as_of","assets":[...],"assets_total","liabilities":[...],"liabilities_total","equity":[...],"equity_total","current_earnings","balanced":bool}`
- `POST /entries` (Fase 1) diperluas: `next_period bool`. Jika true, service mengisi `period_date = period.For(entry_date, startDay).Next` (awal periode berikutnya). Hanya untuk kind `income`.
- `PATCH /api/v1/me` diperluas: `period_start_day *int binding:"omitnil,min=1,max=28"`; `UserResponse` ikut mengembalikan field ini.

**Errors → HTTP**: `date` tidak valid → 400 (parse di handler, `time.Parse("2006-01-02")`); `ErrAccountNotFound` → 404; `ErrNotExpenseAccount` → 400; lainnya 500 via `fail`.

## Frontend
- Routes: `/app/dashboard`, `/app/budgets`, `/app/reports` (toggle Laba Rugi | Neraca via `?view=`). Link "Console" di header → `/app/dashboard`.
- `components/period-picker.tsx`: tombol ‹ › (navigasi ke `prev`/`next` dari response) + label + `<input type="date">` native untuk lompat; state di URL `?date=` (preact-iso `useLocation`), tanpa lib picker.
- `pages/dashboard.tsx`: 4 stat tile (income, expense, surplus, savings rate), daftar transfer ke tabungan, budget bars = `<div role="meter" aria-valuenow aria-valuemax>` dengan warna brand `#0052d9`, merah hanya saat `over`; tabel saldo aset. Tidak ada chart library (progress bar + tabel sudah cukup; tren multi-periode baru butuh chart).
- `pages/budgets.tsx`: tabel akun Expense, input angka (rupiah → sen ×100 saat kirim), badge "diwarisi" jika `inherited`.
- `pages/reports.tsx`: dua tabel sederhana; Neraca menampilkan baris `balanced` hijau/merah.
- Form Pemasukan (Fase 1) tambah checkbox "Hitung untuk periode berikutnya", muncul hanya bila tanggal ≤ 3 hari sebelum tanggal mulai periode.
- `account.tsx`: Field `period_start_day` (`type="number" min=1 max=28`).
- `lib/api.ts`: types `Period, Dashboard, BudgetRow, IncomeStatement, BalanceSheet`; `getDashboard(date?)`, `getBudgets(date?)`, `putBudget(accountId, body)`, `getIncomeStatement(date?)`, `getBalanceSheet(date?)`; `updateProfile` tambah `period_start_day?`. Format uang pakai `fmtIDR` dari `lib/utils.ts` (Fase 1).

## Test
1. `period_test.go` table-driven: (2026-10-16,25)→25 Sep–24 Okt; (2026-10-25,25)→25 Okt–24 Nov; (2026-10-24,25)→25 Sep–24 Okt; (2027-01-10,25)→25 Des 2026–24 Jan 2027 (year boundary); (2026-03-01,28)→28 Feb–27 Mar; (2028-02-28,28)→28 Feb–27 Mar (kabisat); (2026-05-31,1)→1–31 Mei; (2026-10-16,10)→10 Okt–9 Nov (ganti start day).
2. `handler/report_test.go` dashboard: fake `LedgerReader` mengembalikan Gaji 10.000.000 cr Revenue, Bunga 54.000 cr Revenue, Makan 1.200.000 dr Expense, Tabungan 2.000.000 dr Asset kind transfer; fake budget Makan 1.500.000. Assert income 10.054.000, expense 1.200.000 (transfer tidak masuk), surplus 8.854.000, to_savings_total 2.000.000, Makan remaining 300.000 `over=false`.
3. `next_period`: Gaji tanggal 24 Okt dengan `next_period=true` masuk income periode 25 Okt–24 Nov, bukan 25 Sep–24 Okt; saldo Bank per 24 Okt tetap memuatnya.
4. Balance sheet: tambah opening Kas 5.000.000 dr / Modal 5.000.000 cr ke fake; assert `assets_total == liabilities_total + equity_total + current_earnings` dan `balanced=true`.

## Langkah kerja berurutan
1. Migration 000007–000008; `migrate up`.
2. `period` package + test (hijau dulu, murni tanpa DB).
3. `model/report.go` (response structs, `BudgetRequest`), tambah `period_start_day` ke user model/repo/response.
4. `repository/report.go`: `Movements`, `Effective`, `Upsert`.
5. `service/report.go`: interfaces, agregasi Go, 4 method.
6. `handler/report.go` + routes + `fail` + `main.go` wiring.
7. Handler tests dengan fakes.
8. Frontend: `api.ts`, PeriodPicker, Dashboard → Budgets → Reports → field di Account.
9. Cek manual dengan data Excel Oktober: tidak ada defisit palsu.

## Konsep Go yang dipelajari (↔ Laravel)
- `time.Date` menormalkan bulan 0/13 otomatis (≈ Carbon `subMonth` tapi tanpa overflow surprise karena day ≤ 28).
- Table-driven test dengan `[]struct{}` + `t.Run` (≈ PHPUnit `@dataProvider`).
- Interface dipenuhi implisit, dideklarasikan di consumer (≈ Contract, tapi tanpa `implements`).
- Agregasi di Go pakai `map[K]*V` (≈ `collect()->groupBy()->sum()`), money `int64` sen (≈ brick/money, jangan float).
- `DISTINCT ON` Postgres untuk "baris terbaru per grup" (tak ada padanan Eloquent; ganti N+1 `latest()`).
- `c.Query("date")` + parse manual vs `bindJSON` untuk body (≈ `$request->query()` vs FormRequest).

## Risiko & pertanyaan terbuka
- "Hari ini" default: server pakai `time.Now()` UTC; jam 00–07 WIB bisa salah hari. Frontend selalu kirim `?date=` lokal supaya aman.
- Persamaan Neraca hanya tahan jika Fase 1 menjamin Σdebit = Σcredit per entry di service; tidak ada DB constraint lintas baris.
- Ganti `period_start_day` mengubah histori periode; diterima (bukan bug), tetapi perlu catatan di UI.
- `period_date` membuat Laba Rugi dan Neraca memakai tanggal berbeda untuk entry yang sama. Disengaja: Laba Rugi = periode anggaran, Neraca = posisi uang riil.
